package notify

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

const stateFileMode = 0o600

// state is everything the notifier persists. It is treated as a value:
// transitions return a new state and never modify their input.
type state struct {
	// Baseline is set once the transfer history has been read for the first
	// time, so history that predates Crychic is never announced.
	Baseline     bool    `json:"baseline"`
	LastTransfer int     `json:"last_transfer"`
	Watches      []watch `json:"watches"`
}

// watch is one subscription and the people waiting for it.
type watch struct {
	SubscriptionID int          `json:"subscription_id"`
	Source         string       `json:"source"`
	MediaID        string       `json:"media_id"`
	Title          string       `json:"title"`
	Season         *int         `json:"season,omitempty"`
	Start          int          `json:"start,omitempty"`     // first wanted episode
	Total          int          `json:"total,omitempty"`     // last wanted episode, 0 if unknown
	Delivered      []int        `json:"delivered,omitempty"` // episodes arrived so far
	Pending        []int        `json:"pending,omitempty"`   // arrived, not yet announced
	Image          string       `json:"image,omitempty"`     // poster of the latest pending arrival
	ArrivedAt      *time.Time   `json:"arrived_at,omitempty"`
	InactiveSince  *time.Time   `json:"inactive_since,omitempty"`
	Requesters     []flow.Actor `json:"requesters"`
}

// delivery is what one watch's requesters are told after a poll.
type delivery struct {
	watch    watch
	episodes []int
	complete bool
	image    string
}

// withRequest adds req to st, joining an existing watch of the same
// subscription so a late requester is notified too.
func withRequest(st state, req flow.Request) state {
	next := st
	next.Watches = slices.Clone(st.Watches)
	for i, w := range next.Watches {
		if w.SubscriptionID != req.SubscriptionID {
			continue
		}
		if !slices.Contains(w.Requesters, req.Requester) {
			w.Requesters = append(slices.Clone(w.Requesters), req.Requester)
		}
		next.Watches[i] = w
		return next
	}
	t := req.Target
	next.Watches = append(next.Watches, watch{
		SubscriptionID: req.SubscriptionID,
		Source:         t.Media.Source, MediaID: t.Media.ID, Title: t.Media.Title,
		Season: t.Season, Start: max(t.StartEpisode, 1), Total: req.SeasonEpisodes,
		Requesters: []flow.Actor{req.Requester},
	})
	return next
}

// arrive records transfers against the watches without announcing them;
// flush decides when to speak.
func arrive(st state, transfers []Transfer, now time.Time) state {
	next := state{Baseline: st.Baseline, LastTransfer: st.LastTransfer}
	for _, t := range transfers {
		next.LastTransfer = max(next.LastTransfer, t.ID)
	}
	for _, w := range st.Watches {
		next.Watches = append(next.Watches, w.collect(transfers, now))
	}
	return next
}

// collect adds this watch's new arrivals among transfers to its pending ones.
func (w watch) collect(transfers []Transfer, now time.Time) watch {
	for _, t := range transfers {
		if !w.matches(t) {
			continue
		}
		fresh := w.fresh(t.Episodes)
		if w.Season != nil && len(fresh) == 0 {
			continue
		}
		w.Delivered = slices.Sorted(slices.Values(append(slices.Clone(w.Delivered), fresh...)))
		w.Pending = slices.Sorted(slices.Values(append(slices.Clone(w.Pending), fresh...)))
		w.Image, w.ArrivedAt = t.Image, &now
	}
	return w
}

// flush announces watches whose arrivals have settled: everything wanted
// is in, or nothing new came for quiet, so episodes that arrive one by one
// share a notice. A complete watch ends with its notice.
func flush(st state, now time.Time, quiet time.Duration) (state, []delivery) {
	next := state{Baseline: st.Baseline, LastTransfer: st.LastTransfer}
	var out []delivery
	for _, w := range st.Watches {
		complete := w.complete()
		if w.ArrivedAt == nil || (!complete && now.Sub(*w.ArrivedAt) < quiet) {
			next.Watches = append(next.Watches, w)
			continue
		}
		out = append(out, delivery{watch: w, episodes: w.Pending, complete: complete, image: w.Image})
		if !complete {
			w.Pending, w.Image, w.ArrivedAt = nil, "", nil
			next.Watches = append(next.Watches, w)
		}
	}
	return next, out
}

// complete reports whether everything requested has arrived: a movie with
// its first file, a season once every wanted episode is in.
func (w watch) complete() bool {
	if w.Season == nil {
		return w.ArrivedAt != nil
	}
	return w.Total > 0 && len(w.Delivered) >= w.Total-w.Start+1
}

// matches reports whether t is this watch's media (and season, for shows).
func (w watch) matches(t Transfer) bool {
	if t.Source != w.Source || t.MediaID != w.MediaID {
		return false
	}
	if w.Season == nil {
		return t.Season == nil
	}
	return t.Season != nil && *t.Season == *w.Season
}

// fresh filters episodes to wanted ones that have not arrived before.
func (w watch) fresh(episodes []int) []int {
	var out []int
	for _, ep := range episodes {
		if w.wants(ep) && !slices.Contains(w.Delivered, ep) && !slices.Contains(out, ep) {
			out = append(out, ep)
		}
	}
	return out
}

func (w watch) wants(ep int) bool {
	return ep >= w.Start && (w.Total == 0 || ep <= w.Total)
}

// withActivity records which subscriptions MoviePilot no longer has and
// drops watches that have been orphaned for longer than grace. MoviePilot
// closes a subscription when downloads finish, before files are transferred,
// so a closed subscription alone must not end a watch.
func withActivity(st state, active map[int]bool, now time.Time, grace time.Duration) state {
	next := state{Baseline: st.Baseline, LastTransfer: st.LastTransfer}
	for _, w := range st.Watches {
		isActive, checked := active[w.SubscriptionID]
		switch {
		case !checked:
		case isActive:
			w.InactiveSince = nil
		case w.InactiveSince == nil:
			w.InactiveSince = &now
		case now.Sub(*w.InactiveSince) > grace:
			continue
		}
		next.Watches = append(next.Watches, w)
	}
	return next
}

func loadState(path string) (state, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return state{}, nil
	}
	if err != nil {
		return state{}, fmt.Errorf("read notify state: %w", err)
	}
	var st state
	if err := json.Unmarshal(raw, &st); err != nil {
		return state{}, fmt.Errorf("parse notify state %s: %w", path, err)
	}
	return st, nil
}

// saveState writes atomically, so a crash never leaves a torn file.
func saveState(path string, st state) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode notify state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, stateFileMode); err != nil {
		return fmt.Errorf("write notify state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace notify state: %w", err)
	}
	return nil
}
