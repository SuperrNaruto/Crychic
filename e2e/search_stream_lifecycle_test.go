package e2e

import (
	"fmt"
	"testing"
	"time"
)

// A stream started by an initial 下载 response or a typed answer must outlive
// only the request, not the app or an accepted cancellation of that message.
func TestStreamDownloadIntentAndLateInitialResponse(t *testing.T) {
	h := start(t, scenario{cookie: resourceCookie, routes: map[string]route{
		searchPath:  ok(selectedFixture(t, "search_dune.json", 0)),
		duneDetails: ok("detail_dune.json"), duneLookup: ok("subscription_none.json"),
	}})
	first := h.mp.stream(duneStream)
	release := h.tg.holdNextResponse("sendRichMessage")
	defer release()
	h.say(alice, alice, "下载 沙丘")
	first.allow()
	h.wait(first.entered, "search from the initial download-intent response")
	h.tap(alice, 1, "取消")
	h.wait(first.exited, "cancelled stream worker")
	release()
	h.say(alice, alice, "/start")
	h.tap(alice, 2, "搜索")
	second := h.mp.stream(duneStream)
	h.answer(alice, 2, "下载 沙丘")
	second.allow()
	h.wait(second.entered, "search from a typed answer")
	finishStream(h, second, []int{1})
	batchShows(h, 2, "搜索完成")
	h.tap(alice, 2, "返回")
	h.shows(2, "要订阅《沙丘》")
	h.tap(alice, 2, "取消")
	h.shows(1, "已经取消啦")
	h.streamCount(2)
	h.tr.verify(t)
}

// Restore an expired preview neither through old buttons nor a fresh command;
// normal session expiration closes the external stream before a new session.
func TestStreamExpiredSessionClosesItsWorker(t *testing.T) {
	h, s := streamingMovie(t)
	startStream(h, s)
	previewStream(h, s)
	h.tap(alice, 1, "1")
	const beyondSessionTTL = 11 * time.Minute
	h.advance(beyondSessionTTL)
	h.say(alice, alice, "/start")
	h.wait(s.exited, "expired session cancelling the stream")
	h.tap(alice, 1, "刷新结果")
	h.streamCount(1)
	h.tr.verify(t)
}

func TestStreamBatchSelectionCannotReuseEarlierConfirmation(t *testing.T) {
	h, s := streamingMovie(t)
	startStream(h, s)
	finishStream(h, s, []int{0, 1, 2})
	batchShows(h, 1, "搜索完成")
	h.tap(alice, 1, "1")
	stale, _ := findButton(mustMessage(h, 1).rows, downloadIt)
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "做种")
	h.tap(alice, 1, "2")
	h.tapData(alice, 1, stale)
	h.shows(1, "Dune.2021.2160p.WEB-DL") // still the release now shown as 2 after sorting
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "多选…")
	h.tap(alice, 1, "2")
	h.tap(alice, 1, "3")
	h.tap(alice, 1, "下载所选 2 个")
	confirm, _ := findButton(mustMessage(h, 1).rows, downloadIt)
	submitSelected(h, 1, 2)
	h.tapData(alice, 1, confirm)
	h.shows(1, "开始下载 2 个资源")
	h.streamCount(1)
	h.neverShowsSiteCredentials()
	h.tr.verify(t)
}

// Candidate facts may change during final filtering. An accepted detail view
// stays pinned to what was picked and requires an explicit return/reselection.
func TestStreamChangedCandidateKeepsOldDetail(t *testing.T) {
	h, s := streamingMovie(t)
	startStream(h, s)
	previewStream(h, s)
	h.tap(alice, 1, "1")
	items := streamItems(t, []int{0}, false)
	items[0]["torrent_info"].(map[string]any)["size"] = float64(2 << 30)
	h.tr.add(">> Final matching changes the candidate size")
	s.send(h, streamEvent(t, map[string]any{"type": "replace", "items": items, "total_items": 1}))
	s.send(h, streamEvent(t, map[string]any{"type": "done", "stage": "done", "total_items": 1}))
	h.wait(s.exited, "stream completion with changed resource facts")
	h.tap(alice, 1, "刷新结果")
	h.shows(1, "58.2 GiB")
	h.shows(1, "已不在最终结果")
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "1")
	h.shows(1, "2.0 GiB")
	h.tap(alice, 1, downloadIt)
	h.shows(1, "开始下载《沙丘》")
	h.streamCount(1)
	h.tr.verify(t)
}

func TestStreamSearchStaysInsideRequestTopic(t *testing.T) {
	const requestTopic, otherTopic = 42, 77
	h := start(t, scenario{cookie: resourceCookie, routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"), duneLookup: ok("subscription_none.json"),
	}})
	s := h.mp.stream(duneStream)
	h.sayInTopic(alice, requestTopic, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	startStream(h, s)
	previewStream(h, s)
	pick, _ := findButton(mustMessage(h, 1).rows, "1")
	h.sayInTopic(alice, otherTopic, "/start")
	h.tapData(alice, 2, pick)
	h.shows(2, "Crychic")
	finishStream(h, s, []int{1})
	batchShows(h, 1, "搜索完成")
	h.showsInTopic(1, requestTopic)
	h.showsInTopic(2, otherTopic)
	h.tap(alice, 1, "取消")
	h.streamCount(1)
	h.tr.add(fmt.Sprintf(">> stream display remains in request topic %d", requestTopic))
	h.tr.verify(t)
}
