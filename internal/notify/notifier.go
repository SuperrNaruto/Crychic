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
	orphanGrace   = 72 * time.Hour
	activityEvery = time.Hour
)

// Options configures the request fact tracker. It never sends notifications.
type Options struct {
	Feed     Feed
	Path     string
	Interval time.Duration
	Now      func() time.Time
	Log      *slog.Logger
}

// Notifier retains its historical name for the flow.Watcher boundary.
// Its only responsibility is now local request ownership and read-only facts.
type Notifier struct {
	opts          Options
	mu            sync.Mutex
	st            state
	lastActivity  time.Time
	scopeRevision uint64
}

// New loads remembered requests from opts.Path.
func New(opts Options) (*Notifier, error) {
	st, err := loadState(opts.Path)
	if err != nil {
		return nil, err
	}
	st, err = migrateRequestState(opts.Path, st)
	if err != nil {
		return nil, err
	}
	return &Notifier{opts: opts, st: st}, nil
}

// Watch implements flow.Watcher.
func (n *Notifier) Watch(_ context.Context, req flow.Request) error {
	return n.update(func(st state) state { return withRequest(st, req) })
}

// RefreshSubscription preserves existing requesters, arrivals and cursors.
// The caller has observed this identity's current settings after an edit.
func (n *Notifier) RefreshSubscription(_ context.Context, sub flow.Subscription) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.scopeRevision++
	return n.updateLocked(func(st state) state {
		return withSubscriptions(st, map[int]flow.Subscription{sub.ID: sub})
	})
}

// Requested implements flow.Watcher.
func (n *Notifier) Requested(userID int64, subs []flow.Subscription) []int {
	var ids []int
	for _, w := range n.snapshot().Watches {
		if w.Download != "" || !slices.ContainsFunc(w.Requesters, func(r flow.Actor) bool { return r.UserID == userID }) {
			continue
		}
		for _, sub := range subs {
			if w.subscribed(sub) {
				ids = append(ids, sub.ID)
			}
		}
	}
	return ids
}

// subscribed matches both the row id and its target: SQLite can reuse a
// deleted row's id while its old arrival watch is still waiting for files.
func (w watch) subscribed(sub flow.Subscription) bool {
	return w.SubscriptionID == sub.ID && w.media().Kind == sub.Kind &&
		w.sameMedia(watch{Source: sub.Source, MediaID: sub.MediaID, Season: sub.Season})
}

// Forget implements flow.Watcher.
func (n *Notifier) Forget(_ context.Context, sub flow.Subscription) error {
	return n.update(func(st state) state { return without(st, sub) })
}

// ForgetDownload implements flow.Watcher.
func (n *Notifier) ForgetDownload(_ context.Context, id string) error {
	return n.update(func(st state) state { return withoutDownload(st, id) })
}

// Run polls until ctx is cancelled, starting immediately.
func (n *Notifier) Run(ctx context.Context) {
	ticker := time.NewTicker(n.opts.Interval)
	defer ticker.Stop()
	for {
		if err := n.poll(ctx); err != nil && ctx.Err() == nil {
			n.opts.Log.Warn("request fact check failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// poll only reconciles request facts. Native notifications use a separate feed.
func (n *Notifier) poll(ctx context.Context) error {
	snapshot := n.snapshot()
	if !snapshot.Baseline {
		return n.baseline(ctx)
	}
	if err := n.arrivals(ctx, snapshot); err != nil {
		return err
	}
	return n.checkActivity(ctx)
}

func (n *Notifier) baseline(ctx context.Context) error {
	cursor, err := n.opts.Feed.LatestTransfer(ctx)
	if err != nil {
		return err
	}
	return n.update(func(st state) state {
		st.Baseline, st.Cursor = true, &cursor
		for _, id := range cursor.IDs {
			st.LastTransfer = max(st.LastTransfer, id)
		}
		return st
	})
}

// arrivals keeps scope changes and a complete transfer checkpoint atomic.
func (n *Notifier) arrivals(ctx context.Context, snapshot state) error {
	since := ""
	if snapshot.Cursor != nil {
		since = snapshot.Cursor.Date
	}
	transfers, err := n.opts.Feed.TransfersSince(ctx, since)
	if err != nil {
		return err
	}
	snapshot = n.snapshot()
	_, fresh := snapshot.trackTransfers(transfers)
	revision := n.subscriptionScopeRevision()
	subs, err := n.readSubscriptions(ctx, arrivingSubscriptions(snapshot.Watches, fresh))
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return n.updateReadScope(revision, func(st state) state {
		next, fresh := st.trackTransfers(transfers)
		return recordRequestFacts(withSubscriptions(next, subs), fresh, n.opts.Now())
	})
}

// checkActivity asks MoviePilot, at most once per activityEvery, which
// watched subscriptions still exist and which watched downloads are still
// unfinished. The returned subscription must match the watched target:
// an unrelated row reusing its id cannot keep an old watch alive.
func (n *Notifier) checkActivity(ctx context.Context) error {
	now := n.opts.Now()
	if now.Sub(n.lastActivity) < activityEvery {
		return nil
	}
	watches := n.snapshot().Watches
	revision := n.subscriptionScopeRevision()
	active, err := n.downloading(ctx, watches)
	if err != nil {
		return err
	}
	subs, err := n.readSubscriptions(ctx, watches)
	if err != nil {
		return err
	}
	for _, w := range watches {
		if w.Download != "" {
			continue
		}
		active[w.key()] = w.subscribed(subs[w.SubscriptionID])
	}
	err = n.updateReadScope(revision, func(st state) state {
		return withActivity(withSubscriptions(st, subs), active, now)
	})
	if err == nil {
		n.lastActivity = now
	}
	return err
}

// downloading tells, by watch key, which watched downloads are unfinished;
// the downloader is only asked when some watch is for a download.
func (n *Notifier) downloading(ctx context.Context, watches []watch) (map[string]bool, error) {
	active := map[string]bool{}
	if !slices.ContainsFunc(watches, func(w watch) bool { return w.Download != "" }) {
		return active, nil
	}
	downloads, err := n.opts.Feed.Downloads(ctx)
	if err != nil {
		return nil, err
	}
	for _, w := range watches {
		if w.Download != "" {
			active[w.key()] = slices.ContainsFunc(downloads, func(d flow.Download) bool { return d.ID == w.Download })
		}
	}
	return active, nil
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
	return n.updateLocked(transition)
}

// updateLocked is shared by ordinary transitions and checked read commits.
func (n *Notifier) updateLocked(transition func(state) state) error {
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
