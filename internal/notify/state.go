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

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const stateFileMode = 0o600

// state is everything the notifier persists. It is treated as a value:
// transitions return a new state and never modify their input.
type state struct {
	// Baseline excludes history from before Crychic started. LastTransfer
	// is the old id-only checkpoint, used only when Cursor is still nil.
	// A non-nil empty cursor means initialization found no successful files.
	NativeRequests bool            `json:"native_requests,omitempty"`
	Baseline       bool            `json:"baseline"`
	LastTransfer   int             `json:"last_transfer"`
	Cursor         *TransferCursor `json:"transfer_cursor,omitempty"`
	Watches        []watch         `json:"watches"`
	// Digest is every arrival since the last weekly digest; LastDigest is
	// when that went out, or when digests began.
	Digest     []json.RawMessage  `json:"digest,omitempty"`
	LastDigest *time.Time         `json:"last_digest,omitempty"`
	Completed  []completedRequest `json:"completed_requests,omitempty"`
}

// emptied is st without its watches, for a transition to fill them anew;
// everything else carries over.

// watch is one subscription, or one download added by hand, and the people
// waiting for it.
type watch struct {
	SubscriptionID int        `json:"subscription_id"`
	Download       string     `json:"download,omitempty"` // set for a download, then SubscriptionID is 0
	Source         string     `json:"source"`
	MediaID        string     `json:"media_id"`
	Title          string     `json:"title"`
	Year           string     `json:"year,omitempty"`
	Season         *int       `json:"season,omitempty"`
	Start          int        `json:"start,omitempty"`           // first wanted episode
	Total          int        `json:"total,omitempty"`           // latest known last episode, 0 if unknown
	BestVersion    bool       `json:"best_version,omitempty"`    // later versions can deliver the same episode again
	Unmapped       bool       `json:"unmapped,omitempty"`        // observed custom episode group, preserved after closure
	MovieDelivered bool       `json:"movie_delivered,omitempty"` // ordinary movie requests announce once
	Closed         bool       `json:"closed,omitempty"`          // its subscription row was last read gone
	Held           []int      `json:"held,omitempty"`            // existing library episodes, only for downloads
	Delivered      []int      `json:"delivered,omitempty"`       // episodes arrived so far
	Pending        []int      `json:"pending,omitempty"`         // arrived, not yet announced
	Image          string     `json:"image,omitempty"`           // poster of the latest pending arrival
	Releases       []release  `json:"releases,omitempty"`        // one file per download among pending arrivals
	ArrivedAt      *time.Time `json:"arrived_at,omitempty"`
	InactiveSince  *time.Time `json:"inactive_since,omitempty"`
	Stalled        []string   `json:"stalled,omitempty"` // downloads whose stall was told

	Requesters []flow.Actor `json:"requesters"`
}

// release is a download that pending arrivals came from, by one of its files.
type release struct {
	Download string `json:"download,omitempty"`
	File     string `json:"file"`
}

// maxReleases bounds the releases a notice describes.
const maxReleases = 3

func (st state) emptied() state {
	next := st
	next.Watches = nil
	return next
}

func withRequest(st state, req flow.Request) state {
	t := req.Target
	w := watch{
		SubscriptionID: req.SubscriptionID, Download: req.Download,
		Source: t.Media.Source, MediaID: t.Media.ID, Title: t.Media.Title, Year: t.Media.Year,
		Season: t.Season, Start: max(t.StartEpisode, 1), Total: req.SeasonEpisodes,
		BestVersion: req.Download == "" && t.BestVersion,
		Requesters:  []flow.Actor{req.Requester},
	}
	next := st
	next.Watches = slices.Clone(st.Watches)
	for i, current := range next.Watches {
		if !current.same(w) {
			continue
		}
		if current.Closed && current.ArrivedAt == nil {
			break // a new row reusing a finished subscription's id
		}
		current.Requesters = withActors(current.Requesters, w.Requesters)
		current.Total = max(current.Total, w.Total)
		current.BestVersion = current.BestVersion || w.BestVersion
		next.Watches[i] = current
		return next
	}
	// Episodes already in the library never pass through transfer history,
	// yet count towards the season being complete. Downloads keep them
	// separate so replacing an existing episode still produces a notice.
	if req.Download != "" {
		w.Held = slices.Sorted(slices.Values(w.fresh(req.Held)))
	} else {
		w.Delivered = slices.Sorted(slices.Values(w.fresh(req.Held)))
	}
	next.Watches = slices.DeleteFunc(next.Watches, func(o watch) bool {
		return o.same(w) && o.Closed && o.ArrivedAt == nil
	})
	next.Watches = append(next.Watches, w)
	return next
}

func withEpisodes(episodes, more []int) []int {
	all := append(slices.Clone(episodes), more...)
	slices.Sort(all)
	return slices.Compact(all)
}

func (w watch) same(o watch) bool {
	return w.key() == o.key()
}

func (w watch) sameMedia(o watch) bool {
	if w.Source != o.Source || w.MediaID != o.MediaID {
		return false
	}
	if w.Season == nil || o.Season == nil {
		return w.Season == o.Season
	}
	return *w.Season == *o.Season
}

func withActors(actors, more []flow.Actor) []flow.Actor {
	out := slices.Clone(actors)
	for _, a := range more {
		if !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	return out
}

func (w watch) key() string {
	if w.Download != "" {
		return "download:" + w.Download
	}
	season := "movie"
	if w.Season != nil {
		season = fmt.Sprint(*w.Season)
	}
	return fmt.Sprintf("subscription:%d:%q:%q:%s", w.SubscriptionID, w.Source, w.MediaID, season)
}

func (w watch) media() flow.Media {
	m := flow.Media{Source: w.Source, ID: w.MediaID, Title: w.Title, Year: w.Year, Kind: flow.Movie}
	if w.Season != nil {
		m.Kind = flow.TV
	}
	return m
}

func (w watch) complete() bool {
	if w.Season == nil {
		return w.ArrivedAt != nil
	}
	if w.Total == 0 || w.BestVersion || w.Unmapped {
		return false
	}
	for ep := w.Start; ep <= w.Total; ep++ {
		if !slices.Contains(w.Delivered, ep) && !slices.Contains(w.Held, ep) {
			return false
		}
	}
	return true
}

func (w watch) matches(t Transfer) bool {
	if w.Download != "" && t.Download != w.Download {
		return false
	}
	if w.Download == "" && (t.Source != w.Source || t.MediaID != w.MediaID) {
		return false
	}
	if w.Season == nil {
		return t.Season == nil
	}
	return t.Season != nil && *t.Season == *w.Season
}

func (w watch) fresh(episodes []int) []int {
	var out []int
	for _, ep := range episodes {
		if w.wants(ep) && (w.BestVersion || !slices.Contains(w.Delivered, ep)) && !slices.Contains(out, ep) {
			out = append(out, ep)
		}
	}
	return out
}

func (w watch) wants(ep int) bool {
	if w.Download != "" {
		return true
	}
	return ep >= w.Start && (!w.Closed || w.Total == 0 || ep <= w.Total)
}

func without(st state, sub flow.Subscription) state {
	next := st.emptied()
	for _, w := range st.Watches {
		if w.Download != "" || !w.subscribed(sub) {
			next.Watches = append(next.Watches, w)
		}
	}
	return next
}

func withoutDownload(st state, id string) state {
	next := st.emptied()
	for _, w := range st.Watches {
		if w.Download == "" || w.Download != id {
			next.Watches = append(next.Watches, w)
		}
	}
	return next
}

func withActivity(st state, active map[string]bool, now time.Time) state {
	next := st.emptied()
	for _, w := range st.Watches {
		isActive, checked := active[w.key()]
		switch {
		case !checked:
		case isActive:
			w.InactiveSince = nil
		case w.InactiveSince == nil:
			w.InactiveSince = &now
		case now.Sub(*w.InactiveSince) > orphanGrace:
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
