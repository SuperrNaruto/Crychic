package e2e

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	// calendarFixture is a trimmed recording of api.bgm.tv/calendar: every
	// weekday group, a few shows on some of them, only the fields Crychic
	// reads. subject_<id>.json record /v0/subjects/<id> the same way, and
	// not_found.json is Bangumi's answer for a subject it lacks.
	calendarFixture = "calendar.json"
	notFoundFixture = "not_found.json"
	subjectsPath    = "/v0/subjects/"
)

// fakeBangumi serves Bangumi's public API, which needs no key. A subject
// can be made unavailable, as when Bangumi is overloaded.
type fakeBangumi struct {
	*httptest.Server
	t    *testing.T
	tr   *transcript
	mu   sync.Mutex
	down map[string]bool
}

func newFakeBangumi(t *testing.T, tr *transcript) *fakeBangumi {
	f := &fakeBangumi{t: t, tr: tr, down: map[string]bool{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	return f
}

func (f *fakeBangumi) serve(w http.ResponseWriter, r *http.Request) {
	f.tr.add("-> Bangumi " + r.Method + " " + r.URL.Path)
	id, isSubject := strings.CutPrefix(r.URL.Path, subjectsPath)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/calendar":
		f.write(w, http.StatusOK, calendarFixture)
	case r.Method == http.MethodGet && isSubject && f.unavailable(id):
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	case r.Method == http.MethodGet && isSubject:
		f.write(w, http.StatusOK, "subject_"+id+".json")
	default:
		f.write(w, http.StatusNotFound, notFoundFixture)
	}
}

// write answers with a fixture, or Bangumi's not-found answer if there is
// no such fixture.
func (f *fakeBangumi) write(w http.ResponseWriter, status int, name string) {
	body, err := os.ReadFile(filepath.Join("testdata", "bangumi", name))
	if os.IsNotExist(err) {
		status, name = http.StatusNotFound, notFoundFixture
		body, err = os.ReadFile(filepath.Join("testdata", "bangumi", name))
	}
	if err != nil {
		f.t.Error(err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// setDown makes subject id unavailable, or available again.
func (f *fakeBangumi) setDown(id string, down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down[id] = down
}

func (f *fakeBangumi) unavailable(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.down[id]
}
