package notify

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const (
	// orphanGrace is how long a watch outlives its vanished subscription,
	// covering files still being transferred after downloads finished.
	orphanGrace = 72 * time.Hour
	// activityEvery spaces out the per-subscription liveness checks.
	activityEvery      = time.Hour
	libraryConcurrency = 4
	libraryTimeout     = 5 * time.Second
)

// Options configures a Notifier.
type Options struct {
	Feed     Feed
	Sender   Sender
	Path     string // state file
	Interval time.Duration
	Quiet    time.Duration // how long a show's arrivals settle before a notice
	// Destination replaces requester delivery when set; it is an opaque
	// platform address. No requester identity is included in broadcasts.
	Destination string
	// LibraryWait is how long a settled notice waits for the media server
	// to show what arrived; after that it goes out anyway.
	LibraryWait time.Duration
	Now         func() time.Time
	Log         *slog.Logger
}

// Notifier remembers requests and announces arrivals to their requesters.
type Notifier struct {
	opts Options

	mu           sync.Mutex
	st           state
	lastActivity time.Time
}

// New loads remembered requests from opts.Path.
func New(opts Options) (*Notifier, error) {
	st, err := loadState(opts.Path)
	if err != nil {
		return nil, err
	}
	return &Notifier{opts: opts, st: st}, nil
}

// Watch implements flow.Watcher.
func (n *Notifier) Watch(_ context.Context, req flow.Request) error {
	return n.update(func(st state) state { return withRequest(st, req) })
}

// Requested implements flow.Watcher.
func (n *Notifier) Requested(userID int64) []int {
	var ids []int
	for _, w := range n.snapshot().Watches {
		for _, r := range w.Requesters {
			if r.UserID == userID {
				ids = append(ids, w.SubscriptionID)
				break
			}
		}
	}
	return ids
}

// Forget implements flow.Watcher.
func (n *Notifier) Forget(_ context.Context, subscriptionID int) error {
	return n.update(func(st state) state { return without(st, subscriptionID) })
}

// Run polls until ctx is cancelled, starting immediately.
func (n *Notifier) Run(ctx context.Context) {
	ticker := time.NewTicker(n.opts.Interval)
	defer ticker.Stop()
	for {
		if err := n.poll(ctx); err != nil && ctx.Err() == nil {
			n.opts.Log.Warn("arrival check failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// poll reads new transfers and notifies. Network calls run outside the lock;
// state is persisted before sending, so a crash can lose a notice but never
// repeat one.
func (n *Notifier) poll(ctx context.Context) error {
	snapshot := n.snapshot()
	if !snapshot.Baseline {
		latest, err := n.opts.Feed.LatestTransfer(ctx)
		if err != nil {
			return err
		}
		return n.update(func(st state) state {
			return state{Baseline: true, LastTransfer: latest, Watches: st.Watches}
		})
	}
	transfers, err := n.opts.Feed.TransfersAfter(ctx, snapshot.LastTransfer)
	if err != nil {
		return err
	}
	now := n.opts.Now()
	var ready []watch
	err = n.update(func(st state) state {
		next := arrive(st, newerThan(transfers, st.LastTransfer), now)
		ready = due(next, now, n.opts.Quiet)
		return next
	})
	if err != nil {
		return err
	}
	settle := settling{quiet: n.opts.Quiet, held: n.unseen(ctx, ready, now)}
	var deliveries []delivery
	err = n.update(func(st state) state {
		next, out := flush(st, now, settle)
		deliveries = out
		return next
	})
	if err != nil {
		return err
	}
	n.send(ctx, n.withLinks(ctx, deliveries))
	return n.checkActivity(ctx)
}

// unseen picks the ready watches to hold back: the media server does not
// show their arrivals yet and they have waited less than LibraryWait.
func (n *Notifier) unseen(ctx context.Context, ready []watch, now time.Time) map[int]bool {
	held := map[int]bool{}
	if n.opts.LibraryWait <= 0 {
		return held
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, libraryConcurrency)
	for _, w := range ready {
		if n.waited(w, now) >= n.opts.LibraryWait {
			continue
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return held
		}
		wg.Go(func() {
			defer func() { <-slots }()
			if !n.visible(ctx, w) {
				mu.Lock()
				held[w.SubscriptionID] = true
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return held
}

func (n *Notifier) waited(w watch, now time.Time) time.Duration {
	waited := now.Sub(*w.ArrivedAt)
	if !w.complete() {
		waited -= n.opts.Quiet
	}
	return waited
}

// visible asks the media server whether w's pending arrivals show; when it
// cannot tell, the notice is not held up.
func (n *Notifier) visible(ctx context.Context, w watch) bool {
	ctx, cancel := context.WithTimeout(ctx, libraryTimeout)
	defer cancel()
	lib, err := n.opts.Feed.Library(ctx, w.media())
	if err != nil {
		n.opts.Log.Warn("library check failed", "subscription", w.SubscriptionID, "err", err)
		return true
	}
	if w.Season == nil {
		return lib.Movie
	}
	have := lib.Episodes[*w.Season]
	for _, ep := range w.Pending {
		if !slices.Contains(have, ep) {
			return false
		}
	}
	return true
}

// withLinks finds where each delivery can be watched among the media
// servers' newest items.
func (n *Notifier) withLinks(ctx context.Context, deliveries []delivery) []delivery {
	if len(deliveries) == 0 {
		return nil
	}
	items, err := n.opts.Feed.Latest(ctx)
	if err != nil {
		n.opts.Log.Warn("latest items unavailable", "err", err)
		return deliveries
	}
	out := slices.Clone(deliveries)
	for i, d := range out {
		for _, it := range items {
			if it.Title == d.watch.Title && (d.watch.Year == "" || it.Year == d.watch.Year) {
				out[i].watchAt = it
				break
			}
		}
	}
	return out
}

// checkActivity asks MoviePilot, at most once per activityEvery, which
// watched subscriptions still exist.
func (n *Notifier) checkActivity(ctx context.Context) error {
	now := n.opts.Now()
	if now.Sub(n.lastActivity) < activityEvery {
		return nil
	}
	active := map[int]bool{}
	for _, w := range n.snapshot().Watches {
		ok, err := n.opts.Feed.SubscriptionActive(ctx, w.SubscriptionID)
		if err != nil {
			return err
		}
		active[w.SubscriptionID] = ok
	}
	n.lastActivity = now
	return n.update(func(st state) state { return withActivity(st, active, now) })
}

func (n *Notifier) send(ctx context.Context, deliveries []delivery) {
	for _, d := range deliveries {
		text := arrivalText(d)
		recipients := d.watch.Requesters
		if n.opts.Destination != "" {
			recipients = []flow.Actor{{Address: n.opts.Destination}}
		}
		for _, to := range recipients {
			if err := n.opts.Sender.Notify(ctx, flow.Notice{To: to, Text: text, Image: d.image}); err != nil {
				n.opts.Log.Error("arrival notice not delivered", "user", to.UserID, "err", err)
			}
		}
	}
}

func (n *Notifier) snapshot() state {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.st
}

// update applies a transition and persists it before making it current.
func (n *Notifier) update(transition func(state) state) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	next := transition(n.st)
	if reflect.DeepEqual(n.st, next) {
		return nil
	}
	if err := saveState(n.opts.Path, next); err != nil {
		return fmt.Errorf("remember requests: %w", err)
	}
	n.st = next
	return nil
}

func newerThan(transfers []Transfer, id int) []Transfer {
	var out []Transfer
	for _, t := range transfers {
		if t.ID > id {
			out = append(out, t)
		}
	}
	return out
}
