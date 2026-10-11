package notify

import (
	"time"
)

func recordRequestFacts(st state, transfers []Transfer, now time.Time) state {
	next := st.emptied()
	for _, w := range st.Watches {
		updated, changed := requestWatchFacts(w, transfers, now)
		if changed {
			next.Completed = withCompleted(next.Completed, updated, now)
		}
		owned := updated
		owned.Held = nil
		complete := changed && owned.complete() && !owned.Unmapped && !owned.BestVersion
		updated.ArrivedAt = nil
		updated.Pending, updated.Image, updated.Releases, updated.Stalled = nil, "", nil, nil
		if updated.Download != "" && complete {
			continue
		}
		next.Watches = append(next.Watches, updated)
	}
	return next
}

func requestWatchFacts(w watch, transfers []Transfer, now time.Time) (watch, bool) {
	changed := false
	for _, transfer := range transfers {
		if !w.matches(transfer) || (w.Season == nil && w.MovieDelivered) {
			continue
		}
		fresh := w.fresh(transfer.Episodes)
		if w.Season != nil && len(fresh) == 0 {
			continue
		}
		w.Delivered = withEpisodes(w.Delivered, fresh)
		w.MovieDelivered = w.MovieDelivered || w.Season == nil
		w.ArrivedAt, changed = &now, true
	}
	return w, changed
}
