package e2e

import (
	"context"
	"sync"
)

type responseHold struct {
	method string
	done   chan struct{}
	lost   bool
}

// holdNextResponse models a response arriving after Telegram displayed the
// message. The test can press its buttons before the sender learns file_ids.
func (f *fakeTelegram) holdNextResponse(method string) func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	hold := &responseHold{method: method, done: make(chan struct{})}
	f.response = hold
	f.tr.add(">> Telegram delays the next " + method + " response after displaying the message")
	return sync.OnceFunc(func() {
		f.tr.add(">> Telegram releases the delayed " + method + " response")
		close(hold.done)
	})
}

// loseNextResponse displays the message but closes an incomplete HTTP body.
// The caller cannot infer that the server rejected the send.
func (f *fakeTelegram) loseNextResponse(method string) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.response = &responseHold{method: method, done: make(chan struct{}), lost: true}
	return f.response.done
}

func (f *fakeTelegram) waitResponse(ctx context.Context, method string) bool {
	f.mu.Lock()
	hold := f.response
	if hold == nil || hold.method != method {
		f.mu.Unlock()
		return false
	}
	f.response = nil
	f.mu.Unlock()
	if hold.lost {
		f.tr.add("<< " + method + " response lost after display")
		close(hold.done)
		return true
	}
	select {
	case <-hold.done:
	case <-ctx.Done():
	}
	return false
}
