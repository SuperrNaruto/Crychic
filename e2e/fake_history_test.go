package e2e

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	historyPrefix = "/api/v1/subscribe/history/"
	// historyPageSize is MoviePilot's default count for the history route.
	historyPageSize = 30
)

// servesHistory answers a successful subscription history route like
// MoviePilot v3.1.4: the route's fixture is the kind's whole history,
// newest first, served a page at a time (offset (page-1)*count) with the
// whole history's size in X-Total-Count. Failing routes answer as routed.
func (f *fakeMoviePilot) servesHistory(w http.ResponseWriter, r *http.Request) bool {
	path, _ := url.PathUnescape(r.URL.EscapedPath())
	f.mu.Lock()
	rt, routed := f.routes[r.Method+" "+path]
	f.mu.Unlock()
	if r.Method != http.MethodGet || !strings.HasPrefix(path, historyPrefix) || !routed || rt.status != http.StatusOK {
		return false
	}
	var all struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(fixture(f, rt.fixture), &all); err != nil {
		f.t.Errorf("history fixture %s: %v", rt.fixture, err)
	}
	page, count := queryInt(r, "page", 1), queryInt(r, "count", historyPageSize)
	from := min(max(page-1, 0)*count, len(all.Data))
	body, _ := json.Marshal(map[string]any{
		"success": true, "message": "", "data": all.Data[from:min(from+count, len(all.Data))],
	})
	w.Header().Set("X-Total-Count", strconv.Itoa(len(all.Data)))
	writeEnvelope(w, http.StatusOK, string(body))
	return true
}

func queryInt(r *http.Request, name string, fallback int) int {
	n, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return fallback
	}
	return n
}
