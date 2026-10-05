package notify

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

const (
	// orphanGrace is how long a watch outlives its vanished subscription,
	// covering files still being transferred after downloads finished.
	orphanGrace = 72 * time.Hour
	// activityEvery spaces out the per-subscription liveness checks.
	activityEvery = time.Hour
)

// Options configures a Notifier.
type Options struct {
	Feed     Feed
	Sender   Sender
	Path     string // state file
	Interval time.Duration
	Quiet    time.Duration // how long a show's arrivals settle before a notice
	Now      func() time.Time
	Log      *slog.Logger
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
	var deliveries []delivery
	err = n.update(func(st state) state {
		next, out := flush(arrive(st, newerThan(transfers, st.LastTransfer), now), now, n.opts.Quiet)
		deliveries = out
		return next
	})
	if err != nil {
		return err
	}
	n.send(ctx, deliveries)
	return n.checkActivity(ctx)
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
		for _, to := range d.watch.Requesters {
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
