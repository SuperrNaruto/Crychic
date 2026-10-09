package telegram

import (
	"context"

	"github.com/go-telegram/bot/models"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// Initial sends have no message ID until their response returns. Keep only
// navigation accepted while the send is in flight, then discard this record.
type initialReply struct {
	target editTarget
	actor  flow.Actor
	reply  flow.Reply
	moved  map[messageKey]bool
}

func (a *adapter) beginInitial(msg *models.Message, reply flow.Reply) *initialReply {
	if reply.Follow == "" && reply.Input == "" {
		return nil
	}
	to := destinationOf(msg)
	start := &initialReply{
		target: editTarget{chat: msg.Chat.ID, user: msg.From.ID, thread: to.thread},
		actor:  actorOf(*msg.From, to), reply: reply, moved: map[messageKey]bool{},
	}
	a.mu.Lock()
	a.initial[start] = struct{}{}
	a.mu.Unlock()
	return start
}

func (a *adapter) endInitial(start *initialReply) {
	if start == nil {
		return
	}
	a.mu.Lock()
	delete(a.initial, start)
	a.mu.Unlock()
}

// Accepted navigation wins even when its own edit has to be retried. Rejected
// callbacks do not supersede the initial message's input or live view.
func (a *adapter) supersedeInitial(key messageKey) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for start := range a.initial {
		if start.target.chat == key.chat {
			start.moved[key] = true
		}
	}
}

func (a *adapter) completeInitial(ctx context.Context, start *initialReply, message int) {
	if start == nil {
		return
	}
	target := start.target
	target.message = message
	unlock, locked := a.lockMessage(ctx, target.key())
	if !locked {
		return
	}
	defer unlock()
	a.mu.Lock()
	moved := start.moved[target.key()]
	if !moved && start.reply.Input != "" {
		key := inputKey{chat: target.chat, user: target.user, thread: target.thread}
		a.pending[key] = pendingInput{message: message, input: start.reply.Input}
	}
	a.mu.Unlock()
	if !moved && start.reply.Follow != "" {
		a.follow(follower{target: target, actor: start.actor, data: start.reply.Follow, shown: renderReply(start.reply)})
	}
}
