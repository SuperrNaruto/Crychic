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
	Baseline     bool            `json:"baseline"`
	LastTransfer int             `json:"last_transfer"`
	Cursor       *TransferCursor `json:"transfer_cursor,omitempty"`
	Watches      []watch         `json:"watches"`
	// Digest is every arrival since the last weekly digest; LastDigest is
	// when that went out, or when digests began.
	Digest     []digestEntry `json:"digest,omitempty"`
	LastDigest *time.Time    `json:"last_digest,omitempty"`
}

// emptied is st without its watches, for a transition to fill them anew;
// everything else carries over.
func (st state) emptied() state {
	next := st
	next.Watches = nil
	return next
}

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
	MovieDelivered bool       `json:"movie_delivered,omitempty"` // ordinary movie requests announce once
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

// delivery is what one watch's requesters are told after a poll.
type delivery struct {
	watch     watch
	episodes  []int
	complete  bool
	downloads bool // every watch told is a download's
	image     string
	releases  []release
	qualities []string         // the releases' qualities as worded for the notice
	watchAt   flow.LibraryItem // where to watch it; Link is "" when unknown
}

// withRequest adds req to st, joining an existing watch of the same
// subscription target or download so a late requester is notified too.
// A reused subscription id keeps the older target's watch alongside it.
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
		current.Requesters = withActors(current.Requesters, w.Requesters)
		current.Total = max(current.Total, w.Total)
		current.BestVersion = current.BestVersion || w.BestVersion
		next.Watches[i] = current
		return next
	}
	// Episodes already in the library never pass through transfer history,
	// yet count towards the season being complete.
	w.Delivered = slices.Sorted(slices.Values(w.fresh(req.Held)))
	next.Watches = append(next.Watches, w)
	return next
}

// arrive records transfers against the watches without announcing them;
// flush decides when to speak.
func arrive(st state, transfers []Transfer, now time.Time) state {
	next := st.emptied()
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
		if !w.matches(t) || (w.Season == nil && w.MovieDelivered && !w.BestVersion) {
			continue
		}
		fresh := w.fresh(t.Episodes)
		if w.Season != nil && len(fresh) == 0 {
			continue
		}
		w.Delivered = withEpisodes(w.Delivered, fresh)
		w.Pending = withEpisodes(w.Pending, fresh)
		w.MovieDelivered = w.MovieDelivered || w.Season == nil
		w.Image, w.ArrivedAt = t.Image, &now
		w.Releases = w.withRelease(t)
	}
	return w
}

func withEpisodes(episodes, more []int) []int {
	all := append(slices.Clone(episodes), more...)
	slices.Sort(all)
	return slices.Compact(all)
}

// withRelease adds t's download unless one of its files is known already.
func (w watch) withRelease(t Transfer) []release {
	if t.File == "" {
		return w.Releases
	}
	return withRelease(w.Releases, release{Download: t.Download, File: t.File})
}

// withRelease adds r to releases unless its file or download is there
// already, keeping at most maxReleases.
func withRelease(releases []release, r release) []release {
	if len(releases) >= maxReleases {
		return releases
	}
	for _, known := range releases {
		if known.File == r.File || (r.Download != "" && known.Download == r.Download) {
			return releases
		}
	}
	return append(slices.Clone(releases), r)
}

// settled reports whether w's pending arrivals are ready to announce:
// everything wanted is in, or nothing new came for quiet, so episodes that
// arrive one by one share a notice.
func (w watch) settled(now time.Time, quiet time.Duration) bool {
	return w.ArrivedAt != nil && (w.complete() || now.Sub(*w.ArrivedAt) >= quiet)
}

// due lists the watches whose arrivals have settled, with every watch of
// the same media whose pending arrivals their notice would tell too, so
// the media server is asked about all of them.
func due(st state, now time.Time, quiet time.Duration) []watch {
	var out []watch
	for _, w := range st.Watches {
		if !w.settled(now, quiet) {
			continue
		}
		for _, other := range st.Watches {
			if other.pendingFor(w) && !slices.ContainsFunc(out, other.same) {
				out = append(out, other)
			}
		}
	}
	return out
}

// same reports whether w and o are the same watch.
func (w watch) same(o watch) bool {
	return w.key() == o.key()
}

// flush announces the settled watches except those held back (by key)
// until the media server shows them. A complete download ends with its notice;
// subscription requests remain while their subscription is still relevant.
// Watches of the same media (and season) share their arrivals: a
// subscription and a download bringing the same files are told in one
// notice, to everyone who asked for either, once the media server shows
// all of them.
func flush(st state, now time.Time, settle settling) (state, []delivery) {
	watches := slices.Clone(st.Watches)
	ended := make([]bool, len(watches))
	var out []delivery
	for i, w := range watches {
		if ended[i] || !w.settled(now, settle.quiet) || settle.holds(watches, w) {
			continue
		}
		d := delivery{watch: w, complete: true, downloads: true, image: w.Image}
		d.watch.Requesters = nil
		for j, other := range watches {
			if ended[j] || !other.pendingFor(w) {
				continue
			}
			d = d.with(other)
			watches[j], ended[j] = other.announced()
		}
		out = append(out, d)
	}
	next := st.emptied()
	for i, w := range watches {
		if !ended[i] {
			next.Watches = append(next.Watches, w)
		}
	}
	return next, out
}

// holds reports whether w's notice waits: the media server does not show
// the pending arrivals of w or of another watch of the same media yet.
func (s settling) holds(watches []watch, w watch) bool {
	return slices.ContainsFunc(watches, func(o watch) bool {
		return o.pendingFor(w) && s.held[o.key()]
	})
}

func (w watch) pendingFor(other watch) bool {
	return w.ArrivedAt != nil && w.sameMedia(other)
}

// with adds another watch's pending arrivals and requesters to d; the
// notice says all wanted episodes are in only when they are for every
// watch it tells.
func (d delivery) with(w watch) delivery {
	episodes := slices.Clone(d.episodes)
	for _, ep := range w.Pending {
		if !slices.Contains(episodes, ep) {
			episodes = append(episodes, ep)
		}
	}
	slices.Sort(episodes)
	d.episodes = episodes
	d.complete = d.complete && w.complete()
	d.downloads = d.downloads && w.Download != ""
	d.watch.Requesters = withActors(d.watch.Requesters, w.Requesters)
	for _, r := range w.Releases {
		d.releases = withRelease(d.releases, r)
	}
	return d
}

// announced clears pending arrivals. A completed download ends; subscription
// requests retain ownership and can receive added episodes or better versions.
func (w watch) announced() (watch, bool) {
	if w.Download != "" && w.complete() {
		return w, true
	}
	w.MovieDelivered = w.MovieDelivered || w.Season == nil
	w.Pending, w.Image, w.ArrivedAt, w.Releases = nil, "", nil, nil
	return w, false
}

// sameMedia reports whether w and o watch the same media and season.
func (w watch) sameMedia(o watch) bool {
	if w.Source != o.Source || w.MediaID != o.MediaID {
		return false
	}
	if w.Season == nil || o.Season == nil {
		return w.Season == o.Season
	}
	return *w.Season == *o.Season
}

// withActors adds the actors of more not in actors yet.
func withActors(actors, more []flow.Actor) []flow.Actor {
	out := slices.Clone(actors)
	for _, a := range more {
		if !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	return out
}

// settling is how flush decides a watch is ready.
type settling struct {
	quiet time.Duration
	held  map[string]bool // watches (by key) the media server does not show yet
}

// key identifies a watch: its subscription id AND target, or its download.
// Subscription ids are reusable while older files may still be arriving.
// Keys are derived, not persisted, so existing state needs no migration.
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

// media is the watched title as the media server checks know it.
func (w watch) media() flow.Media {
	m := flow.Media{Source: w.Source, ID: w.MediaID, Title: w.Title, Year: w.Year, Kind: flow.Movie}
	if w.Season != nil {
		m.Kind = flow.TV
	}
	return m
}

// complete reports whether everything requested has arrived: a movie with
// its first file, a season once every wanted episode is in. Previously held
// episodes do not prove that a whole season's upgrade has finished.
func (w watch) complete() bool {
	if w.Season == nil {
		return w.ArrivedAt != nil
	}
	if w.Total == 0 || w.BestVersion {
		return false
	}
	for ep := w.Start; ep <= w.Total; ep++ {
		if !slices.Contains(w.Delivered, ep) {
			return false
		}
	}
	return true
}

// matches reports whether t is this watch's media (and season, for shows);
// a download's watch only takes files of that download.
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

// fresh filters wanted episodes. Upgrades accept a new transfer of an episode
// delivered before; the transfer cursor already excludes repeated records.
func (w watch) fresh(episodes []int) []int {
	var out []int
	for _, ep := range episodes {
		if w.wants(ep) && (w.BestVersion || !slices.Contains(w.Delivered, ep)) && !slices.Contains(out, ep) {
			out = append(out, ep)
		}
	}
	return out
}

// wants reports whether ep is asked for. Every file of a download is: the
// download is what was asked for, whatever its season's episode count says.
// A subscription's current total is not a permanent upper bound: ongoing
// seasons gain episodes after the first request.
func (w watch) wants(ep int) bool {
	if w.Download != "" {
		return true
	}
	return ep >= w.Start
}

// without drops only this subscription's target, not an older watch whose
// row id the backend reused for it.
func without(st state, sub flow.Subscription) state {
	next := st.emptied()
	for _, w := range st.Watches {
		if w.Download != "" || !w.subscribed(sub) {
			next.Watches = append(next.Watches, w)
		}
	}
	return next
}

// withoutDownload drops the watch of a download.
func withoutDownload(st state, id string) state {
	next := st.emptied()
	for _, w := range st.Watches {
		if w.Download == "" || w.Download != id {
			next.Watches = append(next.Watches, w)
		}
	}
	return next
}

// withActivity records which subscriptions (or downloads) MoviePilot no
// longer has, by watch key, and drops watches that have been orphaned for
// longer than orphanGrace. MoviePilot closes a subscription when downloads
// finish, and a download leaves the list once it finishes, both before
// files are transferred, so neither alone may end a watch.
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
