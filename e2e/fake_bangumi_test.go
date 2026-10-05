package e2e

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// calendarFixture is a trimmed recording of api.bgm.tv/calendar: every
// weekday group, a few shows on some of them, only the fields Crychic reads.
const calendarFixture = "calendar.json"

// newFakeBangumi serves Bangumi's public calendar, which needs no key.
func newFakeBangumi(t *testing.T, tr *transcript) *httptest.Server {
	body, err := os.ReadFile(filepath.Join("testdata", "bangumi", calendarFixture))
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tr.add("-> Bangumi " + r.Method + " " + r.URL.Path)
		if r.Method != http.MethodGet || r.URL.Path != "/calendar" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}
