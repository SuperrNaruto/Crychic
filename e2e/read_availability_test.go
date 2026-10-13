package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

const bangumiInfoPath = "GET /api/v1/bangumi/"

// MoviePilot owns synopsis lookup; direct Bangumi is only the fallback for
// empty or failed API results, without inventing a TMDB identity.
func TestCalendarPrefersMoviePilotSynopsis(t *testing.T) {
	const secondID = "418229"
	h := start(t, scenario{routes: map[string]route{
		searchPath:                 ok("empty.json"),
		bangumiInfoPath + nuwaID:   ok(writeFixture(t, map[string]any{"overview": "MoviePilot 返回的女娲简介"})),
		bangumiInfoPath + secondID: {status: http.StatusServiceUnavailable, fixture: "server_error.json"},
		bangumiInfoPath + "511936": ok(writeFixture(t, map[string]any{})),
		bangumiInfoPath + "614640": ok(writeFixture(t, nil)),
	}})
	h.bgm.setDown(nuwaID, true)
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "新番放送")
	h.shows(1, "MoviePilot 返回的女娲简介")
	text := h.tr.String()
	if strings.Contains(text, "-> Bangumi GET "+subjectsPath+nuwaID) {
		t.Error("successful MoviePilot synopsis unnecessarily fetched direct Bangumi")
	}
	if !strings.Contains(text, "-> Bangumi GET "+subjectsPath+secondID) {
		t.Error("failed MoviePilot synopsis did not use the direct API fallback")
	}
	for _, id := range []string{"511936", "614640"} {
		if !strings.Contains(text, "-> Bangumi GET "+subjectsPath+id) {
			t.Error("an empty MoviePilot result was treated as a completed synopsis")
		}
	}
	h.shows(1, "bgm.tv/subject/390200")
	h.tr.verify(t)
}

// One slow fallback cannot hold the page past the shared five-second budget.
// Fast annotations remain visible and the unfinished one can be read again.
func TestCalendarEnrichmentHasOneBudget(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{searchPath: ok("empty.json")}})
	const upstreamDelay = 9 * time.Second
	const replyBudget = 7 * time.Second
	entered := h.bgm.delay(nuwaID, upstreamDelay)
	h.say(alice, alice, "/trending")
	began := time.Now()
	done := h.press(alice, 1, "新番放送")
	h.wait(entered, "the slow direct synopsis")
	select {
	case <-done:
	case <-time.After(replyBudget):
		t.Fatal("calendar enrichment exceeded its shared budget")
	}
	elapsed := time.Since(began)
	t.Logf("calendar first reply with %s upstream delay: %s", upstreamDelay, elapsed)
	if elapsed >= replyBudget {
		t.Errorf("calendar reply took %s, budget %s", elapsed, replyBudget)
	}
	h.shows(1, "星期一（今天）")
	if strings.Contains(mustMessage(h, 1).text, "补天的时代已经过去") {
		t.Error("timed-out synopsis was presented as complete")
	}
	h.bgm.delay(nuwaID, 0)
	h.advance(timeoutMissTTL)
	h.say(alice, alice, "/trending")
	h.tap(alice, 2, "新番放送")
	h.shows(2, "补天的时代已经过去")
	h.tr.verify(t)
}

// A missing source must not erase available tasks or pretend everything is
// idle. Retrying stays read-only and rebuilds task buttons from fresh results.
func TestTaskListKeepsAvailableSources(t *testing.T) {
	routes := taskRoutes()
	routes[queuePath] = route{status: http.StatusServiceUnavailable, fixture: "server_error.json"}
	h := start(t, scenario{routes: routes})
	h.say(alice, alice, "/tasks")
	h.shows(1, "迷途之子")
	h.shows(1, "整理状态暂时无法确认")
	h.tap(alice, 1, "1")
	h.shows(1, "0%")
	h.tap(alice, 1, "返回")
	h.mp.setRoute(queuePath, ok("queue_mujica.json"))
	h.mp.setRoute(downloadsPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, 1, "刷新")
	h.shows(1, "颂乐人偶")
	h.shows(1, "下载状态暂时无法确认")
	h.tap(alice, 1, "1")
	h.shows(1, "整理中")
	h.tap(alice, 1, "返回")
	h.mp.setRoute(queuePath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, 1, "刷新")
	h.shows(1, "暂时无法确认")
	if strings.Contains(mustMessage(h, 1).text, "现在没有在下载") {
		t.Error("unavailable task sources were presented as an empty list")
	}
	h.mp.setRoute(downloadsPath, ok("downloads_none.json"))
	h.mp.setRoute(queuePath, ok("queue_none.json"))
	h.tap(alice, 1, "刷新")
	h.shows(1, "现在没有在下载或整理的任务")
	if strings.Contains(h.tr.String(), "-> MoviePilot POST /api/v1/download/") {
		t.Error("read-only task refresh started a download")
	}
	h.tr.verify(t)
}
