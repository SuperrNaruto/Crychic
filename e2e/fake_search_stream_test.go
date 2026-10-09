package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	resourceCookie = "synthetic.resource-cookie.signature"
	duneStream     = "/api/v1/search/media/438631/stream"
	duneSites      = "GET /api/v1/site/media/电影"
	searchSiteMenu = "选择搜索站点"
	streamChunk    = 7 // split UTF-8, field names and JSON across network writes
)

// searchStream controls an external SSE connection, never internal flow state.
// Every write has a flush barrier and closing it models EOF without a done event.
type searchStream struct {
	frames     chan streamFrame
	entered    chan struct{}
	exited     chan struct{}
	status     int
	location   string
	query      string
	mediaType  string
	dropHeader bool
	admission  chan struct{}
	once       sync.Once
}

type streamFrame struct {
	text string
	sent chan struct{}
}

func (f *fakeMoviePilot) stream(path string) *searchStream {
	s := &searchStream{frames: make(chan streamFrame), entered: make(chan struct{}), exited: make(chan struct{}), admission: make(chan struct{})}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.streams == nil {
		f.streams = map[string][]*searchStream{}
	}
	f.streams[path] = append(f.streams[path], s)
	return s
}

func (f *fakeMoviePilot) serveSearchStream(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/v1/search/media/") || !strings.HasSuffix(r.URL.Path, "/stream") {
		return false
	}
	f.mu.Lock()
	queue := f.streams[r.URL.Path]
	if len(queue) == 0 {
		f.mu.Unlock()
		f.record(r)
		f.t.Error("unexpected additional upstream stream request")
		http.Error(w, "unexpected stream", http.StatusBadRequest)
		return true
	}
	s := queue[0]
	f.streams[r.URL.Path] = queue[1:]
	f.mu.Unlock()
	defer close(s.exited)
	select {
	case <-s.admission:
	case <-r.Context().Done():
		return true
	}
	f.record(r)
	close(s.entered)
	f.respondSearchStream(w, r, s)
	return true
}

// Admit after the displayed user action has finished. The worker may have
// connected already, but protocol processing/recording has a repeatable order.
func (s *searchStream) allow() { s.once.Do(func() { close(s.admission) }) }

func (f *fakeMoviePilot) respondSearchStream(w http.ResponseWriter, r *http.Request, s *searchStream) {
	if !f.validateStreamRequest(w, r, s) {
		return
	}
	if s.location != "" {
		http.Redirect(w, r, s.location, http.StatusTemporaryRedirect)
		return
	}
	if s.dropHeader {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			f.t.Error(err)
			return
		}
		conn.Close()
		return
	}
	if s.status != 0 {
		http.Error(w, resourceCookie, s.status)
		return
	}
	typeName := "text/event-stream; charset=utf-8"
	if s.mediaType != "" {
		typeName = s.mediaType
	}
	w.Header().Set("Content-Type", typeName)
	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()
	s.serve(w, r)
}

func (f *fakeMoviePilot) validateStreamRequest(w http.ResponseWriter, r *http.Request, s *searchStream) bool {
	cookie, err := r.Cookie("MoviePilot")
	if err != nil || cookie.Value != resourceCookie || len(r.Cookies()) != 1 {
		f.t.Error("stream did not receive the configured synthetic resource Cookie")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if r.Header.Get("X-API-KEY") != "" || r.Header.Get("Accept") != "text/event-stream" {
		f.t.Error("stream authentication or content negotiation does not match the contract")
	}
	if r.URL.Query().Get("sites") != s.query {
		f.t.Error("stream did not preserve the selected site scope")
	}
	return true
}

func (s *searchStream) serve(w http.ResponseWriter, r *http.Request) {
	for {
		select {
		case <-r.Context().Done():
			return
		case frame, ok := <-s.frames:
			if !ok {
				return
			}
			writeFragments(w, frame.text)
			close(frame.sent)
		}
	}
}

func writeFragments(w http.ResponseWriter, frame string) {
	for len(frame) > 0 {
		n := min(len(frame), streamChunk)
		_, _ = io.WriteString(w, frame[:n])
		w.(http.Flusher).Flush()
		frame = frame[n:]
	}
}

func (s *searchStream) send(h *harness, text string) {
	h.t.Helper()
	frame := streamFrame{text: text, sent: make(chan struct{})}
	select {
	case s.frames <- frame:
	case <-s.exited:
		h.t.Fatal("SSE connection closed before the next frame")
	}
	h.wait(frame.sent, "flushing an SSE frame")
}

func streamEvent(t *testing.T, data map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return "data: " + string(raw) + "\n\n"
}

// Preview contexts follow execution.py: no target media identity until filtering.
func streamItems(t *testing.T, indices []int, preview bool) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "moviepilot", "torrents_dune.json"))
	if err != nil {
		t.Fatal(err)
	}
	var env struct{ Data []map[string]any }
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	out := make([]map[string]any, 0, len(indices))
	for _, i := range indices {
		item := env.Data[i]
		if preview {
			item["media_info"] = nil
			item["media_info_is_target"] = false
			item["match_status"] = "candidate"
		} else {
			item["match_status"] = "exact"
		}
		out = append(out, item)
	}
	return out
}

func assertNoResourceCredentials(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Cookie") != "" {
		t.Error("resource Cookie escaped the allowed SSE request")
	}
}

func (h *harness) streamCount(want int) {
	h.t.Helper()
	got := strings.Count(h.tr.String(), "-> MoviePilot GET "+duneStream)
	if got != want {
		h.t.Errorf("upstream SSE searches: got %d, want %d", got, want)
	}
	if strings.Contains(h.tr.String(), "-> MoviePilot "+duneTorrents+"?") {
		h.t.Error("SSE failure silently restarted search as JSON")
	}
	h.tr.add(fmt.Sprintf(">> upstream SSE searches verified: %d; no JSON fallback", got))
}

func searchableSites(t *testing.T) string {
	t.Helper()
	return writeFixture(t, []map[string]any{
		{"id": 3, "name": "站点甲", "cookie": siteCookie},
		{"id": 7, "name": "站点乙", "apikey": "synthetic-not-for-chat"},
		{"id": 3, "name": "重复配置"}, {"id": 0, "name": "没有身份"},
	})
}
