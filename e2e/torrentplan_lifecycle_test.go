package e2e

import (
	"strings"
	"testing"
)

// A TV-season positive control avoids the movie/no-season guard masking this
// contract: even trusted final candidates are unusable until complete SSE done.
func TestTorrentPlanWaitsForFinalStream(t *testing.T) {
	items := []map[string]any{planRelease(t, planReleaseSpec{name: "FinalTwo", episodes: []int{2}, site: "站点甲", size: planGiB})}
	sc := planScenario(t, items)
	sc.cookie = resourceCookie
	h := start(t, sc)
	first := h.mp.stream(planStream)
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 1 季")
	startStream(h, first)
	preview := planRelease(t, planReleaseSpec{name: "PreviewTwo", episodes: []int{2}, site: "站点甲", size: planGiB})
	preview["media_info"], preview["media_info_is_target"], preview["match_status"] = nil, false, "candidate"
	first.send(h, streamEvent(t, map[string]any{"type": "append", "items": []any{preview}}))
	batchShows(h, 1, "搜索尚未完成")
	planNoButton(h, 1, missingPlan)
	h.tap(alice, 1, "1")
	first.send(h, streamEvent(t, map[string]any{"type": "replace", "stage": "filtered", "items": items, "total_items": len(items)}))
	h.tap(alice, 1, "返回")
	h.shows(1, "搜索尚未完成")
	planNoButton(h, 1, missingPlan)
	close(first.frames)
	batchShows(h, 1, "结果不完整")
	planNoButton(h, 1, missingPlan)
	second := h.mp.stream(planStream)
	h.tap(alice, 1, "重新搜索资源")
	second.allow()
	h.wait(second.entered, "explicit new TV-season stream")
	second.send(h, streamEvent(t, map[string]any{"type": "replace", "stage": "filtered", "items": items, "total_items": len(items)}))
	second.send(h, streamEvent(t, map[string]any{"type": "done", "stage": "done", "total_items": len(items)}))
	h.wait(second.exited, "complete TV-season SSE connection")
	batchShows(h, 1, "搜索完成")
	h.tap(alice, 1, missingPlan)
	h.shows(1, "FinalTwo")
	submitSelected(h, 1, 1)
	if got := strings.Count(h.tr.String(), "-> MoviePilot GET "+planStream); got != 2 {
		t.Errorf("SSE searches = %d, want 2", got)
	}
	if strings.Contains(h.tr.String(), "-> MoviePilot "+mygoTorrents+"?") {
		t.Error("plan or incomplete stream reissued a JSON resource search")
	}
	h.neverShowsSiteCredentials()
	assertResourcePrivacy(h)
	h.tr.verify(t)
}

// Keep a valid, unconsumed preview across rejection and restart. An authorized
// action in the original topic remains a positive control for identity guards.
func TestTorrentPlanIsolationAndRestart(t *testing.T) {
	const requestTopic, otherTopic = 42, 77
	items := []map[string]any{planRelease(t, planReleaseSpec{name: "IsolatedTwo", episodes: []int{2}, site: "站点甲", size: planGiB})}
	h := start(t, planScenario(t, items))
	h.sayInTopic(alice, requestTopic, "/search 迷途之子")
	openPlanResources(h, 1)
	h.tap(alice, 1, missingPlan)
	confirm, found := findButton(mustMessage(h, 1).rows, downloadIt)
	if !found {
		t.Fatal("identity controls require an actually usable plan confirmation")
	}
	original := mustMessage(h, 1).text
	h.tap(bob, 1, downloadIt)
	h.sayInTopic(alice, otherTopic, "/start")
	h.tapData(alice, 2, confirm)
	if mustMessage(h, 1).text != original {
		t.Error("rejected outsider changed the owner's plan")
	}
	h.shows(2, "Crychic")
	h.tap(alice, 1, replan)
	h.shows(1, "IsolatedTwo")
	unconsumed, _ := findButton(mustMessage(h, 1).rows, downloadIt)
	h.showsInTopic(1, requestTopic)
	h.showsInTopic(2, otherTopic)
	h.restart()
	h.tapData(alice, 1, unconsumed)
	h.say(alice, alice, "/tasks")
	planCalls(h, planCallCounts{searches: 1})
	h.neverShowsSiteCredentials()
	h.tr.verify(t)
}
