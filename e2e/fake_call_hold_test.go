package e2e

import (
	"io"
	"net/http"
	"strings"
	"sync"
)

// heldCall suspends an external request before the fake processes it. Tests
// control response order without sleeping or mocking the application's state.
type heldCall struct {
	entered chan struct{}
	release chan struct{}
	exited  chan struct{}
	once    sync.Once
}

func (f *fakeMoviePilot) holdCalls(key string, count int) []*heldCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.held == nil {
		f.held = map[string][]*heldCall{}
	}
	var calls []*heldCall
	for range count {
		call := &heldCall{entered: make(chan struct{}), release: make(chan struct{}), exited: make(chan struct{})}
		calls = append(calls, call)
		f.t.Cleanup(call.allow)
	}
	f.held[key] = append(f.held[key], calls...)
	return calls
}

func (c *heldCall) allow() { c.once.Do(func() { close(c.release) }) }

func (f *fakeMoviePilot) waitCall(r *http.Request) bool {
	key := r.Method + " " + r.URL.Path
	f.mu.Lock()
	calls := f.held[key]
	if len(calls) == 0 {
		f.mu.Unlock()
		return true
	}
	call := calls[0]
	f.held[key] = calls[1:]
	f.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	close(call.entered)
	defer close(call.exited)
	select {
	case <-call.release:
		return true
	case <-r.Context().Done():
		return false
	}
}
