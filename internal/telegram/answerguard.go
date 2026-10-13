package telegram

import (
	"context"
	"sync"
	"time"
)

// answerGuard answers a press without text once its flow has run for
// answerLimit, so the client's spinner stops before Telegram gives up on the
// query. settle ends the guard and reports whether it already answered.
type answerGuard struct {
	cancel chan struct{}
	once   sync.Once
	done   sync.WaitGroup
	early  bool // written by the guard's goroutine, read after done.Wait
}

func (a *adapter) guardAnswer(ctx context.Context, id string) *answerGuard {
	g := &answerGuard{cancel: make(chan struct{})}
	g.done.Go(func() {
		limit := time.NewTimer(a.answerLimit)
		defer limit.Stop()
		select {
		case <-g.cancel:
		case <-limit.C:
			g.early = true
			a.answer(ctx, id, "")
		}
	})
	return g
}

// settle stops the guard, waiting for an answer in flight.
func (g *answerGuard) settle() bool {
	g.once.Do(func() { close(g.cancel) })
	g.done.Wait()
	return g.early
}
