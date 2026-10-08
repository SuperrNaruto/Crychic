package telegram

import (
	"context"

	"github.com/go-telegram/bot/models"
)

type messageLane struct {
	gate  chan struct{}
	users int
}

// Serialize the state transition AND its edit, so a slow old response cannot
// overwrite a newer one. Entries disappear after their last waiter leaves.
func (a *adapter) lockMessage(ctx context.Context, key messageKey) (func(), bool) {
	a.mu.Lock()
	lane := a.lanes[key]
	if lane == nil {
		lane = &messageLane{gate: make(chan struct{}, 1)}
		a.lanes[key] = lane
	}
	lane.users++
	a.mu.Unlock()
	select {
	case lane.gate <- struct{}{}:
		return func() { <-lane.gate; a.leaveLane(key, lane) }, true
	case <-ctx.Done():
		a.leaveLane(key, lane)
		return nil, false
	}
}

func (a *adapter) leaveLane(key messageKey, lane *messageLane) {
	a.mu.Lock()
	defer a.mu.Unlock()
	lane.users--
	if lane.users == 0 {
		delete(a.lanes, key)
	}
}

func callbackKey(cq *models.CallbackQuery) messageKey {
	key := messageKey{chat: callbackChat(cq)}
	if cq.Message.Message != nil {
		key.message = cq.Message.Message.ID
	}
	return key
}

func (a *adapter) stopOwnedFollower(key messageKey, user int64) *follower {
	a.mu.Lock()
	defer a.mu.Unlock()
	if f := a.followers[key]; f != nil && f.actor.UserID == user {
		f.suspended = true
		f.cancel()
		return f
	}
	return nil
}
