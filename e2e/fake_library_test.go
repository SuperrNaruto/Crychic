package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

var (
	seasonLabel  = regexp.MustCompile(`^S(\d+)`)
	episodeLabel = regexp.MustCompile(`^E(\d+)(?:-E(\d+))?`)
)

// libraryQuery is the body of MoviePilot's media server checks.
type libraryQuery struct {
	MediaID string `json:"media_id"`
	Title   string `json:"title"`
}

// servesLibrary answers the media server checks like an Emby that has
// scanned every transfer (unless lagging) on top of what the scenario's
// library fixture held before; it reports whether it handled r.
func (f *fakeMoviePilot) servesLibrary(w http.ResponseWriter, r *http.Request) bool {
	switch r.Method + " " + r.URL.Path {
	case libraryShowPath:
		f.serveEpisodes(w, r)
	case libraryMoviePath:
		f.serveMovie(w, r)
	case latestPath:
		f.serveLatest(w)
	default:
		return false
	}
	return true
}

// shown is the transfers the media server has scanned, newest first.
func (f *fakeMoviePilot) shown() []transfer {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.lagging {
		return append([]transfer(nil), f.transfers...)
	}
	return append([]transfer(nil), f.transfers[len(f.transfers)-f.scanned:]...)
}

func (f *fakeMoviePilot) serveEpisodes(w http.ResponseWriter, r *http.Request) {
	var q libraryQuery
	_ = json.NewDecoder(r.Body).Decode(&q)
	held := map[string][]int{}
	f.heldBefore(libraryShowPath, &held)
	for _, t := range f.shown() {
		if t.MediaID != q.MediaID {
			continue
		}
		if m := seasonLabel.FindStringSubmatch(t.Seasons); m != nil {
			season := strconv.Itoa(atoi(m[1]))
			held[season] = append(held[season], episodesIn(t.Episodes)...)
		}
	}
	data, _ := json.Marshal(held)
	writeEnvelope(w, http.StatusOK, `{"success":true,"message":"","data":`+string(data)+`}`)
}

func (f *fakeMoviePilot) serveMovie(w http.ResponseWriter, r *http.Request) {
	var q libraryQuery
	_ = json.NewDecoder(r.Body).Decode(&q)
	for _, t := range f.shown() {
		if t.MediaID == q.MediaID {
			writeEnvelope(w, http.StatusOK, string(fixture(f, "library_movie_held.json")))
			return
		}
	}
	f.mu.Lock()
	rt, found := f.routes[libraryMoviePath]
	f.mu.Unlock()
	if !found {
		rt = ok("library_movie_missing.json")
	}
	writeEnvelope(w, rt.status, string(fixture(f, rt.fixture)))
}

// serveLatest lists scanned transfers as the server's newest items,
// unless the scenario gives a recording.
func (f *fakeMoviePilot) serveLatest(w http.ResponseWriter) {
	f.mu.Lock()
	rt, found := f.routes[latestPath]
	f.mu.Unlock()
	if found {
		writeEnvelope(w, rt.status, string(fixture(f, rt.fixture)))
		return
	}
	var items []map[string]any
	seen := map[string]bool{}
	for _, t := range f.shown() {
		if seen[t.MediaID] {
			continue
		}
		seen[t.MediaID] = true
		items = append(items, map[string]any{
			"id": t.MediaID, "title": t.Title, "subtitle": t.Year, "type": t.Type,
			"link": fmt.Sprintf("https://emby.example.com/web/index.html#!/item?id=%s", t.MediaID),
		})
	}
	data, _ := json.Marshal(items)
	if items == nil {
		data = []byte("[]")
	}
	writeEnvelope(w, http.StatusOK, `{"success":true,"message":"","data":`+string(data)+`}`)
}

// heldBefore merges the scenario's library fixture for key into held.
func (f *fakeMoviePilot) heldBefore(key string, held *map[string][]int) {
	f.mu.Lock()
	rt, found := f.routes[key]
	f.mu.Unlock()
	if !found {
		return
	}
	var env struct {
		Data map[string][]int `json:"data"`
	}
	_ = json.Unmarshal(fixture(f, rt.fixture), &env)
	for season, eps := range env.Data {
		(*held)[season] = append((*held)[season], eps...)
	}
}

// catchUp makes a lagging media server scan every transfer so far.
func (f *fakeMoviePilot) catchUp() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scanned = len(f.transfers)
}

func fixture(f *fakeMoviePilot, name string) []byte {
	data, err := os.ReadFile(filepath.Join("testdata", "moviepilot", name))
	if err != nil {
		f.t.Errorf("fixture: %v", err)
	}
	return data
}

func episodesIn(label string) []int {
	m := episodeLabel.FindStringSubmatch(label)
	if m == nil {
		return nil
	}
	first, last := atoi(m[1]), atoi(m[1])
	if m[2] != "" {
		last = atoi(m[2])
	}
	var eps []int
	for ep := first; ep <= last; ep++ {
		eps = append(eps, ep)
	}
	return eps
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
