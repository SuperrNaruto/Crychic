package flow

import (
	"context"
	"slices"
	"sync"
	"time"
)

// torrentHunt is a resource search running in the background, so the
// conversation can say how long it has been searching instead of holding
// the message until the sites answer.
type torrentHunt struct {
	mu        sync.Mutex
	started   time.Time
	cancel    context.CancelFunc
	done      chan struct{}
	first     chan struct{}
	streaming bool
	base      int // first stable index allocated to this generation
	token     int // independent of result counts, including empty generations
	items     []Torrent
	order     []int
	finished  bool
	final     bool
	err       error
	terminal  *TorrentSnapshot // withheld until the request returns and closes
}

type huntProgress struct {
	items []Torrent
	order []int
	done  bool
	final bool
	err   error
}

// The worker changes snapshots, never session screens or confirmations.
func (e *Engine) hunt(sess session) *torrentHunt {
	ctx, cancel := context.WithCancel(e.runCtx)
	h := &torrentHunt{
		started: e.now(), cancel: cancel, done: make(chan struct{}), first: make(chan struct{}),
		streaming: e.backend.StreamsTorrents(), base: len(sess.torrents), token: sess.hunts,
	}
	search := TorrentSearch{Target: *sess.focus, Sites: slices.Clone(sess.searchSites)}
	e.work.Go(func() {
		defer cancel()
		var err error
		if h.streaming {
			err = e.backend.StreamTorrents(ctx, search, h.publish)
		} else {
			var found []Torrent
			found, err = e.backend.SearchTorrents(ctx, search)
			h.publish(TorrentSnapshot{Items: found, Final: err == nil})
		}
		h.finish(err)
	})
	return h
}

func (h *torrentHunt) publish(snapshot TorrentSnapshot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if snapshot.Final {
		h.terminal = &snapshot
		return
	}
	h.apply(snapshot)
	if len(h.items) > 0 {
		h.wakeFirst()
	}
}

// apply runs under mu; terminal facts and terminal state are published together.
func (h *torrentHunt) apply(snapshot TorrentSnapshot) {
	indices := make(map[string]int, len(h.items))
	for i, item := range h.items {
		indices[item.Identity] = i
	}
	h.order = nil
	for _, item := range snapshot.Items {
		i, ok := indices[item.Identity]
		if !ok {
			i = len(h.items)
			indices[item.Identity] = i
			h.items = append(h.items, item)
		} else {
			h.items[i] = item
		}
		h.order = append(h.order, h.base+i)
	}
	h.final = snapshot.Final
}

func (h *torrentHunt) finish(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err == nil && h.terminal != nil {
		h.apply(*h.terminal)
	}
	h.terminal = nil
	h.err, h.finished = err, true
	close(h.done)
	h.wakeFirst()
}

func (h *torrentHunt) wakeFirst() {
	select {
	case <-h.first:
	default:
		close(h.first)
	}
}

func (h *torrentHunt) read() huntProgress {
	h.mu.Lock()
	defer h.mu.Unlock()
	return huntProgress{slices.Clone(h.items), slices.Clone(h.order), h.finished, h.final, h.err}
}

// Stop waits for the local worker; it cannot promise remote site cancellation.
func (h *torrentHunt) stop() {
	if h != nil {
		h.cancel()
		<-h.done
	}
}
