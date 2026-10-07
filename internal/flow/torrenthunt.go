package flow

import (
	"context"
	"time"
)

// torrentHunt is a resource search running in the background, so the
// conversation can say how long it has been searching instead of holding
// the message until the sites answer.
type torrentHunt struct {
	started time.Time
	cancel  context.CancelFunc
	done    chan struct{} // closed once found and err are set
	found   []Torrent
	err     error
}

// hunt starts searching the sites for target. Its context is the engine's
// own: the search outlives the button press that started it, and ends when
// the backend answers (bounded by the backend's timeout) or stop is called.
func (e *Engine) hunt(target Target) *torrentHunt {
	ctx, cancel := context.WithCancel(context.Background())
	h := &torrentHunt{started: e.now(), cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(h.done)
		h.found, h.err = e.backend.SearchTorrents(ctx, target)
	}()
	return h
}

// stop abandons the search, or releases a finished one; nil is fine.
func (h *torrentHunt) stop() {
	if h != nil {
		h.cancel()
	}
}
