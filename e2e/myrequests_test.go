package e2e

import (
	"strings"
	"testing"
	"time"
)

const requestPrivate = "DO_NOT_SHOW_REQUEST_DIAGNOSTICS"

// This is the owner of the personal-list contract. Existing subscription
// detail tests own general per-episode rendering; here the records must
// originate with this chat user, not the shared administrator account.
func TestMyRequestsOwnershipAndProgress(t *testing.T) {
	h := start(t, scenario{routes: myRequestRoutes(t)})
	requestBreakingSeason(h)
	installRequestProgress(h)
	before := h.tr.String()
	h.say(alice, group, "/requests")
	h.shows(2, "我的请求")
	h.shows(2, "《绝命毒师》第 2 季")
	h.tap(bob, 2, "1")
	h.tap(alice, 2, "1")
	h.shows(2, "请求详情")
	h.shows(2, "MoviePilot 待获取 13 集")
	h.shows(2, "下载中 37%")
	h.shows(2, "已入库")
	h.shows(2, "整理中")
	h.tap(alice, 2, "返回")
	h.tap(alice, 2, "需留意")
	h.shows(2, "没有发现需留意的请求")
	h.tap(alice, 2, "跟踪中")
	checkUncertainRequestProgress(h)
	h.say(bob, bob, "/requests")
	h.shows(3, "还没有通过我登记的请求")
	h.say(bob, bob, "/start")
	h.tap(bob, 4, "我的请求")
	h.shows(4, "<h3>🎬 Crychic</h3>")
	h.say(alice, alice, "/start")
	h.tap(alice, 5, "我的请求")
	h.shows(5, "《绝命毒师》第 2 季")
	assertRequestReadsOnly(h, before)
	assertRequestPrivate(h)
	h.tr.verify(t)
}

func checkUncertainRequestProgress(h *harness) {
	h.t.Helper()
	sub := breakingSeason(1, 2)
	sub["execution_status"] = map[string]any{"state": requestPrivate}
	h.mp.setRoute(subsPath, ok(writeFixture(h.t, []map[string]any{sub})))
	download := requestDownload("requested-hash")
	download["media"].(map[string]any)["season"] = ""
	h.mp.setRoute(downloadsPath, ok(writeFixture(h.t, []map[string]any{download})))
	h.tap(alice, 2, "需留意")
	h.shows(2, "《绝命毒师》第 2 季")
	h.tap(alice, 2, "1")
	h.shows(2, "执行状态未知")
	h.shows(2, "季或集数不明")
}

// Removing a tracked request cannot reuse its list number for the next
// request, nor let a previously visible detail keep reading on its behalf.
func TestMyRequestsRefreshKeepsTargets(t *testing.T) {
	h := requestedBreakingSeasons(t)
	h.say(alice, alice, "/requests")
	old, _ := findButton(mustMessage(h, 3).rows, "1")
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 4, "取消订阅")
	h.tap(alice, 4, "1")
	h.tap(alice, 4, "确认取消")
	before := h.tr.String()
	h.tap(alice, 3, "刷新")
	h.shows(3, "《绝命毒师》第 3 季")
	h.tapData(alice, 3, old)
	h.tap(alice, 3, "2")
	h.shows(3, "《绝命毒师》第 3 季")
	h.tap(alice, 3, "返回")
	h.tap(alice, 3, "刷新")
	h.tapData(alice, 3, old)
	const beyondRequestTTL = 11 * time.Minute
	h.advance(beyondRequestTTL)
	h.tap(alice, 3, "2")
	h.shows(3, "/requests")
	assertRequestReadsOnly(h, before)
	h.tr.verify(t)
}

func myRequestRoutes(t *testing.T) map[string]route {
	t.Helper()
	return map[string]route{
		searchPath: ok("search_breaking_bad.json"), breakDetails: ok("detail_breaking_bad.json"),
		seasonsPath: ok("seasons_breaking_bad.json"), breakingQuery: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"), subsPath: ok(writeFixture(t, []map[string]any{breakingSeason(1, 2)})),
	}
}

func requestBreakingSeason(h *harness) {
	h.t.Helper()
	h.say(alice, group, "/search 绝命毒师")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 2 季")
	h.tap(alice, 1, "从第 1 集开始")
}

func installRequestProgress(h *harness) {
	h.t.Helper()
	h.mp.setRoute(libraryShowPath, ok(writeFixture(h.t, map[string][]int{"2": {1}})))
	h.mp.setRoute(downloadsPath, ok(writeFixture(h.t, []map[string]any{requestDownload("requested-hash")})))
	h.mp.setRoute(queuePath, ok(writeFixture(h.t, []map[string]any{{
		"media":  map[string]any{"media_source": "themoviedb", "media_id": "1396", "type": "电视剧", "title": requestPrivate},
		"season": 2, "tasks": []map[string]any{{"state": "running", "meta": map[string]any{"begin_episode": 2}}},
	}})))
}

func requestDownload(hash string) map[string]any {
	return map[string]any{
		"hash": hash, "downloader": requestPrivate, "progress": 37, "state": "downloading",
		"media": map[string]any{"media_source": "themoviedb", "media_id": "1396", "type": "电视剧",
			"title": requestPrivate, "season": "S02", "episode": "E03-E13"},
	}
}

func assertRequestReadsOnly(h *harness, before string) {
	h.t.Helper()
	after := strings.TrimPrefix(h.tr.String(), before)
	for _, forbidden := range []string{"-> MoviePilot POST", "-> MoviePilot PUT", "-> MoviePilot PATCH", "-> MoviePilot DELETE", "GET /api/v1/search/", "/api/v1/system/message", "/manual_review"} {
		if strings.Contains(after, forbidden) {
			h.t.Errorf("request browsing unexpectedly invoked %s", forbidden)
		}
	}
}

func assertRequestPrivate(h *harness) {
	h.t.Helper()
	if strings.Contains(h.tr.String(), requestPrivate) {
		h.t.Error("private diagnostics reached the chat or transcript")
	}
}
