package e2e

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

const mpAPIKey = "mp-test-key"

// route is a canned MoviePilot answer. Fixture bodies follow the MoviePilot
// v3.1.0 response models (app/schemas/context.py, subscribe.py).
type route struct {
	status  int
	fixture string
}

func ok(fixture string) route { return route{status: http.StatusOK, fixture: fixture} }

// newFakeMoviePilot serves routes keyed by "METHOD /path" and enforces the
// X-API-KEY header the way MoviePilot does.
func newFakeMoviePilot(t *testing.T, tr *transcript, routes map[string]route) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, _ := url.PathUnescape(r.URL.EscapedPath())
		key := r.Method + " " + path
		query, _ := url.QueryUnescape(r.URL.RawQuery)
		head := "-> MoviePilot " + key
		if query != "" {
			head += "?" + query
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) > 0 {
			tr.add(head, string(body))
		} else {
			tr.add(head)
		}
		if r.Header.Get("X-API-KEY") != mpAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"detail":"apikey 校验不通过"}`)
			return
		}
		rt, found := routes[key]
		if !found {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"detail":"Not Found"}`)
			return
		}
		data, err := os.ReadFile(filepath.Join("testdata", "moviepilot", rt.fixture))
		if err != nil {
			t.Errorf("fixture: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.status)
		_, _ = w.Write(data)
	}))
}
