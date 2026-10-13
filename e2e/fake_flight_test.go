package e2e

import (
	"net/http"
	"strings"
	"sync"
)

// flightGauge counts the lookups of calendar notes (media searches and
// Bangumi details) MoviePilot has in progress at once, held ones included,
// and the most it has seen together.
type flightGauge struct {
	mu   sync.Mutex
	now  int
	peak int
}

func isNoteLookup(r *http.Request) bool {
	path := r.URL.Path
	return r.Method == http.MethodGet && (path == "/api/v1/media/search" || strings.HasPrefix(path, "/api/v1/bangumi/"))
}

// enter counts r if it is a note lookup; the returned func ends it.
func (g *flightGauge) enter(r *http.Request) func() {
	if !isNoteLookup(r) {
		return func() {}
	}
	g.mu.Lock()
	g.now++
	g.peak = max(g.peak, g.now)
	g.mu.Unlock()
	return func() {
		g.mu.Lock()
		g.now--
		g.mu.Unlock()
	}
}

func (g *flightGauge) mostAtOnce() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.peak
}
