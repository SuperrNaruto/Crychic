package flow

import (
	"slices"
	"sync"
	"time"
)

const (
	// noteTTL is how long a calendar pick's found synopsis and identity are
	// remembered; noteMissTTL how long a finished lookup that found no
	// synopsis is, so such a pick is not looked up on every page view.
	noteTTL     = 6 * time.Hour
	noteMissTTL = time.Hour
	// noteTimeoutTTL is how long a pick whose lookup the round's budget cut
	// short is remembered as a miss: MoviePilot keeps working on a search
	// after the client gave up, so retrying at once piles up its load.
	noteTimeoutTTL = 10 * time.Minute
	// noteCacheMax bounds the remembered picks; the oldest go first.
	noteCacheMax = 512
	// defaultNoteGrace is how long a calendar page waits for its notes; they
	// are cosmetic and never hold a page back for longer.
	defaultNoteGrace = 800 * time.Millisecond
)

// noteEntry is what a finished lookup made of one calendar pick.
type noteEntry struct {
	pick Media
	at   time.Time
	ttl  time.Duration
}

func (n noteEntry) fresh(now time.Time) bool {
	return now.Sub(n.at) < n.ttl
}

// noteRound is one background lookup covering several picks; done closes
// when it has finished, however it ended.
type noteRound struct{ done chan struct{} }

// noteResult is one pick's outcome of a round. A lookup the round's budget
// cut short is timedOut and remembered briefly; one the app's run ended is
// neither done nor timedOut and is never remembered.
type noteResult struct {
	pick     Media
	done     bool
	miss     bool
	timedOut bool
}

// ttl is how long the result is remembered.
func (r noteResult) ttl() time.Duration {
	switch {
	case r.timedOut:
		return noteTimeoutTTL
	case r.miss:
		return noteMissTTL
	}
	return noteTTL
}

// noteCache is the Engine's memory of calendar picks' notes, keyed by the
// calendar's own id, and of the rounds looking them up. Sessions only read
// it when they render; background rounds are the only writers.
type noteCache struct {
	mu      sync.Mutex
	entries map[string]noteEntry
	order   []string // keys, oldest first
	flying  map[string]*noteRound
}

func newNoteCache() *noteCache {
	return &noteCache{entries: map[string]noteEntry{}, flying: map[string]*noteRound{}}
}

// apply is pick with its remembered note: the TMDB identity, link and
// synopsis, keeping the calendar's air date, weekday, id and rating.
func (c *noteCache) apply(pick Media, now time.Time) (Media, bool) {
	if pick.CalendarID == "" {
		return pick, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	n, ok := c.entries[pick.CalendarID]
	if !ok || !n.fresh(now) {
		return pick, false
	}
	m := n.pick
	m.Released, m.Weekday, m.CalendarID = pick.Released, pick.Weekday, pick.CalendarID
	if pick.Rating > 0 {
		m.Rating = pick.Rating
	}
	return m, true
}

// stale tells whether pick needs a lookup: it has none remembered.
func (c *noteCache) stale(pick Media, now time.Time) bool {
	if pick.CalendarID == "" {
		return false
	}
	n, ok := c.entries[pick.CalendarID]
	return !ok || !n.fresh(now)
}

// claim is the lookups picks need: the picks no round is looking up yet,
// which belong to a new round, and every round covering a stale pick.
type noteClaim struct {
	mine     []Media
	round    *noteRound
	covering []*noteRound
}

func (c *noteCache) claim(picks []Media, now time.Time) noteClaim {
	c.mu.Lock()
	defer c.mu.Unlock()
	var cl noteClaim
	for _, p := range picks {
		if !c.stale(p, now) {
			continue
		}
		if r, ok := c.flying[p.CalendarID]; ok {
			if !slices.Contains(cl.covering, r) {
				cl.covering = append(cl.covering, r)
			}
			continue
		}
		if cl.round == nil {
			cl.round = &noteRound{done: make(chan struct{})}
			cl.covering = append(cl.covering, cl.round)
		}
		c.flying[p.CalendarID] = cl.round
		cl.mine = append(cl.mine, p)
	}
	return cl
}

// active are the rounds still looking up picks that remain stale.
func (c *noteCache) active(picks []Media, now time.Time) []*noteRound {
	c.mu.Lock()
	defer c.mu.Unlock()
	var rounds []*noteRound
	for _, p := range picks {
		r, ok := c.flying[p.CalendarID]
		if ok && c.stale(p, now) && !slices.Contains(rounds, r) {
			rounds = append(rounds, r)
		}
	}
	return rounds
}

// settle remembers one pick's finished lookup, so pages see it before the
// round's slower picks are done, and stops counting the pick as being looked up.
func (c *noteCache) settle(r noteResult, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := r.pick.CalendarID
	delete(c.flying, key)
	if r.done || r.timedOut {
		c.put(key, noteEntry{pick: r.pick, at: now, ttl: r.ttl()})
	}
}

func (c *noteCache) put(key string, n noteEntry) {
	if _, ok := c.entries[key]; !ok {
		c.order = append(c.order, key)
		if len(c.order) > noteCacheMax {
			delete(c.entries, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.entries[key] = n
}
