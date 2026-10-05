package telegram

import (
	"context"
	"time"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

// messageKey identifies one message the bot may keep refreshing.
type messageKey struct {
	chat    int64
	message int
}

func (t editTarget) key() messageKey { return messageKey{chat: t.chat, message: t.message} }

// follower keeps a live reply (flow.Reply.Follow) on screen up to date.
type follower struct {
	target editTarget
	actor  flow.Actor
	data   string // the Follow data of the reply on screen
	shown  string // rendered text on screen, to skip edits that change nothing
	cancel context.CancelFunc
}

// follow starts refreshing f's message, replacing any earlier follower of it.
func (a *adapter) follow(f follower) {
	ctx, cancel := context.WithCancel(a.runCtx)
	f.cancel = cancel
	key := f.target.key()
	a.mu.Lock()
	if old := a.followers[key]; old != nil {
		old.cancel()
	}
	a.followers[key] = &f
	a.mu.Unlock()
	go a.refresh(ctx, &f)
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
	unlock, ok := a.lockMessage(ctx, f.target.key())
	if !ok {
		return false
	}
	defer unlock()
	if ctx.Err() != nil {
		return false
	}
	reply := a.flow.Choose(ctx, f.actor, f.data)
	if ctx.Err() != nil || reply.Notice != "" {
		return false
	}
	if rich := renderRich(reply.Text, reply.Image); rich != f.shown {
		a.edit(ctx, f.target, reply)
		f.shown = rich
	}
	f.data = reply.Follow
	return reply.Follow != ""
}

// forget drops f from the followers unless a newer one replaced it.
func (a *adapter) forget(f *follower) {
	f.cancel()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.followers[f.target.key()] == f {
		delete(a.followers, f.target.key())
	}
}
