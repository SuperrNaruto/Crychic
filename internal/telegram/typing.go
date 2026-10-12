package telegram

import (
	"context"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	// defaultTypingLead is how long a command or title search runs before
	// the chat shows "typing…", so quick answers never flash it.
	defaultTypingLead = 300 * time.Millisecond
	// typingEvery renews the action: Telegram shows it for 5 seconds at most.
	typingEvery = 4 * time.Second
)

// typing shows "typing…" in msg's chat and topic once the work for a new
// message has run for typingLead, renewing it until the returned stop is
// called. stop waits for a call still in flight, so the action never lands
// after the reply it precedes. A negative typingLead shows nothing.
func (a *adapter) typing(ctx context.Context, msg *models.Message) (stop func()) {
	if a.typingLead < 0 {
		return func() {}
	}
	to := destinationOf(msg)
	done, exited := make(chan struct{}), make(chan struct{})
	a.work.Go(func() {
		defer close(exited)
		a.typeUntil(ctx, to, done)
	})
	return func() {
		close(done)
		<-exited
	}
}

// typeUntil sends the action after typingLead and every typingEvery, until
// done closes or ctx ends. It never holds a lane, and finishes the call it
// is making before returning.
func (a *adapter) typeUntil(ctx context.Context, to destination, done <-chan struct{}) {
	timer := time.NewTimer(a.typingLead)
	defer timer.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		_, err := a.api.SendChatAction(ctx, &bot.SendChatActionParams{
			ChatID: to.chat, MessageThreadID: to.thread, Action: models.ChatActionTyping,
		})
		a.logFailure("sendChatAction", err)
		timer.Reset(typingEvery)
	}
}
