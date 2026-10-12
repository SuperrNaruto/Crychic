package telegram

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const (
	// byteCacheEntries and byteCacheBytes bound the downloaded hotlinked
	// posters kept until Telegram has shown them and given a file_id.
	byteCacheEntries = 32
	byteCacheBytes   = 16 << 20
	// prefetchFetches is how many posters download ahead at once, apart from
	// the posterFetches a message being sent uses.
	prefetchFetches = 2
	// warmBudget is how long a button press waits for its reply's posters
	// before answering, so the client's spinner covers them.
	warmBudget = 2 * time.Second
)

// byteEntry is one downloaded poster.
type byteEntry struct {
	url  string
	data []byte
}

// flight is one poster download under way; later requests for the same URL
// wait for it instead of downloading again.
type flight struct {
	done chan struct{}
	data []byte
	err  error
}

// posterBytes holds downloaded posters (an LRU bounded by count and size,
// successes only) and the downloads in flight. Downloads run on the bot's
// own context, not on any caller's, so a caller giving up early (a press
// answering after warmBudget) leaves the download for the next one to join.
type posterBytes struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List // most recently used first
	size    int
	flights map[string]*flight

	base context.Context // ends the downloads when the bot stops
	work *sync.WaitGroup // counts the downloads, which Bot.Run waits for
}

func newPosterBytes() *posterBytes {
	return &posterBytes{
		entries: map[string]*list.Element{}, order: list.New(), flights: map[string]*flight{},
		base: context.Background(), work: &sync.WaitGroup{},
	}
}

// bind ties downloads to the bot's run: they end with ctx and work waits
// for them.
func (p *posters) bind(ctx context.Context, work *sync.WaitGroup) {
	p.bytes.mu.Lock()
	defer p.bytes.mu.Unlock()
	p.bytes.base, p.bytes.work = ctx, work
}

// download returns the poster at raw, from the cache, an earlier download
// still under way, or a new one; it gives up waiting when ctx ends.
func (p *posters) download(ctx context.Context, raw string) ([]byte, error) {
	f, data, hit := p.bytes.join(raw, p.fetchInto)
	if hit {
		return data, nil
	}
	select {
	case <-f.done:
		return f.data, f.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// join finds raw cached, or in flight, or starts its download with run.
func (b *posterBytes) join(raw string, run func(context.Context, string, *flight)) (*flight, []byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if el, ok := b.entries[raw]; ok {
		b.order.MoveToFront(el)
		return nil, el.Value.(byteEntry).data, true
	}
	if f := b.flights[raw]; f != nil {
		return f, nil, false
	}
	f := &flight{done: make(chan struct{})}
	b.flights[raw] = f
	b.work.Add(1)
	go run(b.base, raw, f)
	return f, nil, false
}

// fetchInto runs one download for f and caches what it got.
func (p *posters) fetchInto(base context.Context, raw string, f *flight) {
	defer p.bytes.work.Done()
	ctx, cancel := context.WithTimeout(base, posterBudget)
	defer cancel()
	f.data, f.err = p.fetch(ctx, raw)
	if f.err != nil {
		p.log.Warn("poster download failed", "image", raw, "err", f.err)
	}
	p.bytes.finish(raw, f)
	close(f.done)
}

func (b *posterBytes) finish(raw string, f *flight) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.flights, raw)
	if f.err != nil {
		return
	}
	b.entries[raw] = b.order.PushFront(byteEntry{url: raw, data: f.data})
	b.size += len(f.data)
	for b.order.Len() > byteCacheEntries || b.size > byteCacheBytes {
		b.removeLocked(b.order.Back())
	}
}

// drop forgets raw's bytes once Telegram has given it a file_id.
func (b *posterBytes) drop(raw string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if el, ok := b.entries[raw]; ok {
		b.removeLocked(el)
	}
}

func (b *posterBytes) removeLocked(el *list.Element) {
	e := el.Value.(byteEntry)
	b.order.Remove(el)
	delete(b.entries, e.url)
	b.size -= len(e.data)
}

// warm downloads reply's hotlinked posters into the cache, waiting at most
// warmBudget; one still downloading then is left to finish.
func (p *posters) warm(ctx context.Context, reply flow.Reply) {
	urls := p.missing(shownPosters(reply))
	if len(urls) == 0 {
		return
	}
	p.fetchWithin(ctx, urls, warmBudget)
}

// prefetch downloads hotlinked posters ahead of their being shown, a few
// at a time, until all are done or the bot stops.
func (p *posters) prefetch(urls []string) {
	todo := p.missing(dedupe(urls))
	if len(todo) == 0 {
		return
	}
	queue := make(chan string, len(todo))
	for _, raw := range todo {
		queue <- raw
	}
	close(queue)
	p.bytes.mu.Lock()
	base, work := p.bytes.base, p.bytes.work
	p.bytes.mu.Unlock()
	for range min(prefetchFetches, len(todo)) {
		work.Go(func() {
			for raw := range queue {
				if base.Err() != nil {
					return
				}
				_, _ = p.download(base, raw)
			}
		})
	}
}

func dedupe(urls []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range urls {
		if !seen[raw] {
			seen[raw] = true
			out = append(out, raw)
		}
	}
	return out
}
