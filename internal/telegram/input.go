package telegram

import (
	"context"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// Private text and commands share an input lane (message zero is not a
// Telegram message), so a new command cannot race an older typed answer.
func (a *adapter) lockInput(ctx context.Context, msg *models.Message) (func(), bool) {
	if msg.Chat.Type != models.ChatTypePrivate {
		return func() {}, true
	}
	return a.lockMessage(ctx, messageKey{chat: msg.Chat.ID})
}

func refused(user int64) flow.Reply {
	return flow.Reply{Text: flow.Lines(
		flow.Heading(flow.Plain("🚫 没有权限")),
		flow.Line(
			flow.Plain("抱歉，你还没有使用权限哦。把你的 Telegram ID 发给管理员吧："),
			flow.Mono(strconv.FormatInt(user, decimal)),
		),
	)}
}

// replyTo also remembers input requested by an initial response, not only
// by an edited message.
func (a *adapter) replyTo(ctx context.Context, msg *models.Message, reply flow.Reply) {
	sent, err := a.send(ctx, msg.Chat.ID, reply)
	a.logFailure("send reply", err)
	if err == nil && reply.Input != "" {
		a.mu.Lock()
		a.pending[inputKey{chat: msg.Chat.ID, user: msg.From.ID}] = pendingInput{message: sent.ID, input: reply.Input}
		a.mu.Unlock()
	}
}

// onText gives a pending question priority over a new private title search.
// Group messages only answer a prompt when they quote it.
func (a *adapter) onText(ctx context.Context, msg *models.Message) {
	if strings.TrimSpace(msg.Text) == "" {
		return
	}
	if !a.allowed[msg.From.ID] {
		if msg.Chat.Type == models.ChatTypePrivate {
			a.replyTo(ctx, msg, refused(msg.From.ID))
		}
		return
	}
	key := inputKey{chat: msg.Chat.ID, user: msg.From.ID}
	a.mu.Lock()
	p, waiting := a.pending[key]
	a.mu.Unlock()
	if waiting {
		a.answerText(ctx, msg, p)
		return
	}
	if msg.Chat.Type == models.ChatTypePrivate {
		reply := a.flow.Start(ctx, actorOf(*msg.From, msg.Chat.ID), msg.Text)
		a.replyTo(ctx, msg, reply)
	}
}

func (a *adapter) answerText(ctx context.Context, msg *models.Message, p pendingInput) {
	key := inputKey{chat: msg.Chat.ID, user: msg.From.ID}
	unlock, locked := a.lockMessage(ctx, messageKey{chat: key.chat, message: p.message})
	if !locked {
		return
	}
	defer unlock()
	a.mu.Lock()
	current := a.pending[key]
	a.mu.Unlock()
	if current != p {
		return
	}
	isReply := msg.ReplyToMessage != nil && msg.ReplyToMessage.ID == p.message
	if msg.Chat.Type != models.ChatTypePrivate && !isReply {
		return
	}
	reply := a.flow.Answer(ctx, actorOf(*msg.From, msg.Chat.ID), flow.Typed{Input: p.input, Text: msg.Text})
	if reply.Notice != "" {
		a.log.Warn("typed answer refused", "notice", reply.Notice)
		return
	}
	a.edit(ctx, editTarget{chat: key.chat, user: key.user, message: p.message}, reply)
}
