package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Every invalid protocol case has the same user contract: preserve uncertainty,
// never submit a resource or reconnect. Frames are synthetic upstream input.
func TestStreamProtocolFailuresAndEmptyCompletion(t *testing.T) {
	h, first := streamingMovie(t)
	cases := []struct{ name, frame string }{
		{"error with sensitive text", streamEvent(t, map[string]any{"type": "error", "message": resourceCookie})},
		{"truncated frame", "data: {\"type\":\"done\",\"stage\":\"done\",\"items\":[],\"total_items\":0}"},
		{"done without final result", streamEvent(t, map[string]any{"type": "done", "stage": "done", "total_items": 0})},
		{"out of order batch", streamEvent(t, map[string]any{"type": "replace", "items": []any{}, "replace_batch": true, "batch_index": 1, "batch_count": 2})},
		{"wrong total", "data: {\"type\":\"replace\",\"items\":[],\"total_items\":1}\n\ndata: {\"type\":\"done\",\"stage\":\"done\",\"total_items\":1}\n\n"},
		{"invalid JSON", "data: {not valid JSON}\n\n"},
		{"missing final replace batch", "data: {\"type\":\"replace\",\"items\":[],\"replace_batch\":true,\"batch_index\":0,\"batch_count\":2}\n\ndata: {\"type\":\"done\",\"stage\":\"done\"}\n\n"},
		{"done carrying an incomplete batch", "data: {\"type\":\"done\",\"stage\":\"done\",\"items\":[],\"replace_batch\":true,\"batch_index\":0,\"batch_count\":2}\n\n"},
	}
	for i, tc := range cases {
		s := first
		if i > 0 {
			s = h.mp.stream(duneStream)
			h.tap(alice, 1, "重新搜索资源")
		} else {
			startStream(h, s)
		}
		s.allow()
		h.wait(s.entered, "an explicitly initiated stream")
		h.tr.add(">> SSE protocol case: " + tc.name)
		s.send(h, tc.frame)
		close(s.frames)
		batchShows(h, 1, "结果不完整")
	}
	last := h.mp.stream(duneStream)
	h.tap(alice, 1, "重新搜索资源")
	last.allow()
	h.wait(last.entered, "the explicit empty-result search")
	h.tr.add(">> SSE returns a complete, genuinely empty result")
	finishStream(h, last, []int{})
	batchShows(h, 1, "没有匹配的资源")
	h.streamCount(len(cases) + 1)
	if strings.Contains(h.tr.String(), resourceCookie) || strings.Contains(h.tr.String(), "-> MoviePilot "+downloadPath) {
		t.Fatal("invalid protocol exposed credentials or submitted a download")
	}
	h.tap(alice, 1, "取消")
	h.tr.verify(t)
}

func TestStreamRedirectAndLostHeadersNeverReplay(t *testing.T) {
	h, first := streamingMovie(t)
	var redirected atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
		assertNoResourceCredentials(t, r)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer sink.Close()
	for i, kind := range []string{"cross-origin redirect", "same-origin non-stream redirect", "lost response headers", "JSON instead of SSE"} {
		s := first
		if i > 0 {
			s = h.mp.stream(duneStream)
		}
		switch i {
		case 0:
			s.location = sink.URL + "/collect"
		case 1:
			s.location = h.mp.URL + "/api/v1/media/438631"
		case 2:
			s.dropHeader = true
		case 3:
			s.mediaType = "application/json"
		}
		h.tr.add(">> SSE transport case: " + kind)
		if i == 0 {
			startStream(h, s)
		} else {
			h.tap(alice, 1, "重新搜索资源")
		}
		s.allow()
		h.wait(s.exited, "closing the single failed request")
		batchShows(h, 1, "结果不完整")
	}
	if redirected.Load() != 0 {
		t.Error("search redirects must not be followed, even without credentials")
	}
	h.streamCount(4)
	h.tap(alice, 1, "取消")
	h.tr.verify(t)
}

// Waiting the real two-minute budget keeps production free of test-only timer
// knobs. Preview details are pinned, so no wall-clock progress enters the golden.
func TestStreamTotalBudgetClosesIncompleteSearch(t *testing.T) {
	h, s := streamingMovie(t)
	startStream(h, s)
	previewStream(h, s)
	h.tap(alice, 1, "1")
	const searchBudgetAllowance = 130 * time.Second
	select {
	case <-s.exited:
	case <-time.After(searchBudgetAllowance):
		t.Fatal("resource search outlived its total read budget")
	}
	h.tr.add(">> The total resource-search deadline closes the silent connection")
	h.tap(alice, 1, "刷新结果")
	h.shows(1, "结果不完整")
	h.tap(alice, 1, "取消")
	h.streamCount(1)
	h.tr.verify(t)
}

func TestStreamFirstAndFinalResultsWakeBeforeRefreshInterval(t *testing.T) {
	h := start(t, scenario{cookie: resourceCookie, progress: "1h", routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"), duneLookup: ok("subscription_none.json"),
	}})
	s := h.mp.stream(duneStream)
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	startStream(h, s)
	previewStream(h, s) // the external wait is five seconds, not the one-hour cadence
	h.tr.add(">> Final result wakes the display without waiting for its one-hour cadence")
	finishStream(h, s, []int{1})
	batchShows(h, 1, "搜索完成")
	h.shows(1, fmt.Sprintf("共 %d 个", 1))
	h.tap(alice, 1, "取消")
	h.streamCount(1)
	h.tr.verify(t)
}
