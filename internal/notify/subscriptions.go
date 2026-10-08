package notify

import (
	"context"
	"fmt"
	"slices"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// arrivingSubscriptions are the subscriptions whose new or pending arrivals
// need current episode counts and upgrade settings before completion is judged.
func arrivingSubscriptions(watches []watch, transfers []Transfer) []watch {
	var out []watch
	for _, w := range watches {
		if w.Download == "" && (w.ArrivedAt != nil || slices.ContainsFunc(transfers, w.matches)) {
			out = append(out, w)
		}
	}
	return out
}

// readSubscriptions reads each row once, including a successful empty row for
// a subscription that finished before its files arrived. A failed read leaves
// the arrival cursor and pending state untouched so the next poll can retry.
func (n *Notifier) readSubscriptions(ctx context.Context, watches []watch) (map[int]flow.Subscription, error) {
	subs := map[int]flow.Subscription{}
	for _, w := range watches {
		if w.Download != "" {
			continue
		}
		if _, read := subs[w.SubscriptionID]; read {
			continue
		}
		sub, err := n.opts.Feed.Subscription(ctx, w.SubscriptionID)
		if err != nil {
			return nil, fmt.Errorf("read arrival subscription %d: %w", w.SubscriptionID, err)
		}
		subs[w.SubscriptionID] = sub
	}
	return subs, nil
}

// withSubscriptions refreshes only matching live targets and marks the
// others closed. A completed or replaced row cannot erase the last known
// scope or upgrade mode of files still on their way, and concurrent new
// requests are retained.
func withSubscriptions(st state, subs map[int]flow.Subscription) state {
	next := st
	next.Watches = slices.Clone(st.Watches)
	for i, w := range next.Watches {
		sub, read := subs[w.SubscriptionID]
		if w.Download != "" || !read {
			continue
		}
		w.Closed = !w.subscribed(sub)
		next.Watches[i] = w
		if w.Closed {
			continue
		}
		if sub.Total > 0 {
			w.Total = sub.Total
		}
		w.BestVersion = sub.BestVersion
		next.Watches[i] = w
	}
	return next
}
