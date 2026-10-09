package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func streamingMovie(t *testing.T) (*harness, *searchStream) {
	t.Helper()
	h := start(t, scenario{cookie: resourceCookie, routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"),
		duneLookup: ok("subscription_none.json"), downloadPath: ok("download_added.json"),
	}})
	s := h.mp.stream(duneStream)
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	return h, s
}

func startStream(h *harness, s *searchStream) {
	h.t.Helper()
	h.tap(alice, 1, searchResources)
	s.allow()
	h.wait(s.entered, "one upstream SSE search")
}

func previewStream(h *harness, s *searchStream) {
	h.t.Helper()
	h.tr.add(">> MoviePilot streams unverified candidates, including one duplicate")
	s.send(h, "\ufeff: heartbeat\r\n\r\ndata: {\"type\":\rdata: \"heartbeat\"}\r\r")
	s.send(h, streamEvent(h.t, map[string]any{
		"type": "append", "stage": "searching", "items": streamItems(h.t, []int{0, 1, 0}, true),
	}))
	batchShows(h, 1, "搜索尚未完成")
	h.shows(1, "共 2 个")
}

func finishStream(h *harness, s *searchStream, indices []int) {
	h.t.Helper()
	s.send(h, streamEvent(h.t, map[string]any{
		"type": "replace", "stage": "filtered", "items": streamItems(h.t, indices, false), "total_items": len(indices),
	}))
	s.send(h, streamEvent(h.t, map[string]any{"type": "done", "stage": "done", "total_items": len(indices)}))
	h.wait(s.exited, "closing a completed SSE connection")
}

// Early candidates lack media identity. They are readable, never downloadable;
// final filtering may remove them or reorder the list without reusing numbers.
func TestStreamSearchKeepsIdentityAndWaitsForFinalMatch(t *testing.T) {
	h, s := streamingMovie(t)
	startStream(h, s)
	previewStream(h, s)
	h.tap(alice, 1, "1")
	h.shows(1, "Dune.2021.2160p.UHD")
	if _, ok := findButton(mustMessage(h, 1).rows, downloadIt); ok {
		t.Fatal("unverified preview exposed a download confirmation")
	}
	h.tap(bob, 1, "刷新结果")
	h.tr.add(">> MoviePilot replaces the result set in two batches")
	s.send(h, streamEvent(t, map[string]any{
		"type": "replace", "items": streamItems(t, []int{1}, false), "replace_batch": true,
		"batch_index": 0, "batch_count": 2, "total_items": 2,
	}))
	h.tap(alice, 1, "刷新结果")
	h.shows(1, "搜索尚未完成")
	s.send(h, streamEvent(t, map[string]any{
		"type": "append", "items": streamItems(t, []int{2}, false), "replace_batch": true,
		"batch_index": 1, "batch_count": 2, "total_items": 2,
	}))
	s.send(h, streamEvent(t, map[string]any{"type": "done", "stage": "done", "total_items": 2}))
	h.wait(s.exited, "SSE terminal connection closure")
	h.tap(alice, 1, "刷新结果")
	h.shows(1, "已不在最终结果")
	h.tap(alice, 1, "返回")
	h.shows(1, `<li value="2"><b>Dune.2021.1080p`)
	h.tap(alice, 1, "2")
	confirm, _ := findButton(mustMessage(h, 1).rows, downloadIt)
	h.tap(alice, 1, downloadIt)
	h.tapData(alice, 1, confirm)
	h.shows(1, "开始下载《沙丘》啦")
	h.streamCount(1)
	h.neverShowsSiteCredentials()
	assertResourcePrivacy(h)
	h.tr.verify(t)
}

// A completed selection is frozen by its canonical resource record, not its
// position. A later search must never make an old confirmation write a new one.
func TestStreamSearchRetriesOnlyExplicitlyAndRecoversDelivery(t *testing.T) {
	h, s := streamingMovie(t)
	startStream(h, s)
	previewStream(h, s)
	h.tr.add(">> MoviePilot disconnects in the middle of a final replacement")
	s.send(h, streamEvent(t, map[string]any{
		"type": "replace", "items": streamItems(t, []int{1}, false), "replace_batch": true,
		"batch_index": 0, "batch_count": 2, "total_items": 2,
	}))
	failed := h.tg.failNextEdit(1)
	close(s.frames)
	h.wait(failed, "a failed terminal search edit")
	batchShows(h, 1, "结果不完整")
	h.shows(1, "可能重复访问站点")
	h.streamCount(1)
	next := h.mp.stream(duneStream)
	h.tap(alice, 1, "重新搜索资源")
	next.allow()
	h.wait(next.entered, "an explicitly requested new search")
	finishStream(h, next, []int{1})
	batchShows(h, 1, "按 MoviePilot 的优先级")
	h.tap(alice, 1, "返回")
	h.shows(1, "要订阅《沙丘》")
	last := h.mp.stream(duneStream)
	startStream(h, last)
	finishStream(h, last, []int{1})
	batchShows(h, 1, "搜索完成")
	h.tap(alice, 1, "4") // earlier generations used 1–3; navigation cannot recycle them
	h.tap(alice, 1, downloadIt)
	h.shows(1, "开始下载《沙丘》啦")
	h.streamCount(3)
	h.neverShowsSiteCredentials()
	h.tr.verify(t)
}

func assertResourcePrivacy(h *harness) {
	h.t.Helper()
	if strings.Contains(h.tr.String(), resourceCookie) {
		h.t.Fatal("resource Cookie reached the transcript")
	}
	raw, err := os.ReadFile(filepath.Join(h.cfg.DataDir, "requests.json"))
	if err != nil {
		h.t.Fatal(err)
	}
	if strings.Contains(string(raw), resourceCookie) || strings.Contains(string(raw), siteCookie) {
		h.t.Fatal("resource credentials were persisted")
	}
}

// Authentication errors contain deliberately secret-looking upstream text.
// Metadata remains available and neither auth response causes another search.
func TestStreamCookieFailuresKeepJSONMetadataAvailable(t *testing.T) {
	h, s := streamingMovie(t)
	s.status = http.StatusUnauthorized
	startStream(h, s)
	batchShows(h, 1, "资源 Cookie")
	h.shows(1, "不会自动重新搜索")
	h.streamCount(1)
	second := h.mp.stream(duneStream)
	second.status = http.StatusForbidden
	h.tap(alice, 1, "重新搜索资源")
	second.allow()
	h.wait(second.entered, "a second explicit attempt after Cookie rejection")
	batchShows(h, 1, "资源 Cookie")
	h.say(alice, alice, "/search 沙丘")
	h.shows(2, "帮你找到这些")
	checkCookieOtherServices(h)
	h.tap(alice, 1, "取消")
	h.streamCount(2)
	if strings.Contains(h.tr.String(), resourceCookie) {
		t.Fatal("authentication failure leaked the resource Cookie")
	}
	h.tr.verify(t)
}

func checkCookieOtherServices(h *harness) {
	h.t.Helper()
	h.mp.setRoute(doubanTVPath, ok("chart_douban_tv.json"))
	h.say(alice, alice, "/trending")
	h.tap(alice, 3, "豆瓣热门剧集") // actual poster downloads through the shared app wiring
	h.shows(3, "豆瓣热门剧集")
	h.tap(alice, 3, "返回")
	h.tap(alice, 3, "新番放送") // the public Bangumi service receives no Cookie either
	h.tap(alice, 3, "关闭")
}

func TestStreamSearchCancelsOnNavigationAndRestart(t *testing.T) {
	h, s := streamingMovie(t)
	staleConfirm, _ := findButton(mustMessage(h, 1).rows, "确认订阅")
	startStream(h, s)
	h.tapData(alice, 1, staleConfirm)
	previewStream(h, s)
	oldPick, _ := findButton(mustMessage(h, 1).rows, "1")
	h.tap(alice, 1, "返回")
	h.wait(s.exited, "navigation closing the upstream stream")
	h.shows(1, "要订阅《沙丘》")
	next := h.mp.stream(duneStream)
	startStream(h, next)
	h.tapData(alice, 1, oldPick)
	h.restart()
	h.wait(next.exited, "shutdown closing and waiting for the stream worker")
	h.say(alice, alice, "/tasks")
	h.tap(alice, 1, "取消")
	h.streamCount(2)
	h.tr.verify(t)
}
