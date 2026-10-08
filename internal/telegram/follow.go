package telegram

import (
	"context"
	"errors"
	"time"

	"github.com/go-telegram/bot"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// messageKey identifies one message the bot may keep refreshing.
type messageKey struct {
	chat    int64
	message int
}

func (t editTarget) key() messageKey { return messageKey{chat: t.chat, message: t.message} }

// follower keeps a live reply (flow.Reply.Follow) on screen up to date.
type follower struct {
	target     editTarget
	actor      flow.Actor
	data       string // the Follow data of the reply on screen
	shown      string // rendered text on screen, to skip edits that change nothing
	cancel     context.CancelFunc
	pending    *flow.Reply // a flow transition already read but not delivered
	retryAt    time.Time
	retryUntil time.Time
	suspended  bool // protected by adapter.mu; a callback may resume a rejected choice
}

// follow starts refreshing f's message, replacing any earlier follower of it.
func (a *adapter) follow(f follower) {
	ctx, cancel := context.WithCancel(a.runCtx)
	f.cancel, f.suspended = cancel, false
	key := f.target.key()
	a.mu.Lock()
	if old := a.followers[key]; old != nil {
		old.cancel()
	}
	a.followers[key] = &f
	a.mu.Unlock()
	// Only a handler follows, so the work it is part of is still counted.
	a.work.Go(func() { a.refresh(ctx, &f) })
}

// unfollow stops refreshing a message, e.g. because its owner pressed a
// button on it; that press decides what the message shows next.
func (a *adapter) unfollow(key messageKey) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if f := a.followers[key]; f != nil {
		f.cancel()
		delete(a.followers, key)
	}
}

// refresh asks the flow for the reply's next state every followEvery until
// the flow ends it or the follower is cancelled.
func (a *adapter) refresh(ctx context.Context, f *follower) {
	defer a.forget(f)
	ticker := time.NewTicker(a.followEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if !a.refreshOnce(ctx, f) {
			return
		}
	}
}

func (a *adapter) refreshOnce(ctx context.Context, f *follower) bool {
	now := time.Now()
	if !f.retryUntil.IsZero() && !now.Before(f.retryUntil) {
		return false
	}
	if now.Before(f.retryAt) {
		return true
	}
	unlock, ok := a.lockMessage(ctx, f.target.key())
	if !ok {
		return false
	}
	defer unlock()
	if ctx.Err() != nil {
		return false
	}
	if f.pending == nil {
		reply := a.flow.Choose(ctx, f.actor, f.data)
		if reply.Notice != "" {
			return false
		}
		f.pending = &reply
	}
	if ctx.Err() != nil {
		return false
	}
	return a.deliverFollow(ctx, f)
}

// deliverFollow retries only the cached reply, never the transition that
// produced it. A terminal result still belongs here until it is visible.
func (a *adapter) deliverFollow(ctx context.Context, f *follower) bool {
	reply := *f.pending
	rich := renderReply(reply)
	if rich != f.shown {
		if _, err := a.edit(ctx, f.target, reply); err != nil {
			f.deferRetry(err)
			return ctx.Err() == nil
		}
		f.shown = rich
	}
	f.pending = nil
	f.retryAt, f.retryUntil = time.Time{}, time.Time{}
	f.data = reply.Follow
	return reply.Follow != ""
}

// Telegram's flood-control delay overrides the normal refresh cadence.
// Other failed edits retry on that cadence within the follower's budget.
func (f *follower) deferRetry(err error) {
	if f.retryUntil.IsZero() {
		f.retryUntil = time.Now().Add(flow.FollowFor)
	}
	var limited *bot.TooManyRequestsError
	if errors.As(err, &limited) && limited.RetryAfter > 0 {
		f.retryAt = time.Now().Add(time.Duration(limited.RetryAfter) * time.Second)
	}
}

// forget drops f from the followers unless a newer one replaced it.
func (a *adapter) forget(f *follower) {
	f.cancel()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.followers[f.target.key()] == f && !f.suspended {
		delete(a.followers, f.target.key())
	}
}

// resumeFollower runs under the message lane, after the cancelled refresh
// stopped mutating f. A newer navigation/follower invalidates its identity;
// otherwise its unsent reply survives without repeating the transition.
func (a *adapter) resumeFollower(f *follower) {
	if f == nil {
		return
	}
	a.mu.Lock()
	current := a.followers[f.target.key()] == f
	copy := *f
	a.mu.Unlock()
	if current {
		a.follow(copy)
	}
}

func (a *adapter) discardFollower(f *follower) {
	if f == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.followers[f.target.key()] == f {
		delete(a.followers, f.target.key())
	}
}
