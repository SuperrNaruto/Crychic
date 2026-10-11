package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const (
	batchResources = 3
	stopBatch      = "停止后续提交"
)

func pendingBatch(t *testing.T) *harness {
	t.Helper()
	h := start(t, scenario{stall: "0s", routes: map[string]route{
		searchPath: ok("search_mygo.json"), mygoDetails: ok("detail_mygo.json"),
		seasonsPath: ok("seasons_mygo.json"), mygoLookup: ok("subscription_none.json"),
		mygoTorrents: searchingSites("torrents_mygo_s1.json"), downloadPath: ok("download_added.json"),
	}})
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 1 季")
	h.searchTorrents(alice, 1, searchResources)
	h.tap(alice, 1, "多选…")
	for _, label := range []string{"2", "3", "4"} {
		h.tap(alice, 1, label)
	}
	h.tap(alice, 1, "下载所选 3 个")
	return h
}

// Keep receipt expectations behind real displayed edits, not guessed sleeps.
func batchShows(h *harness, message int, want string) {
	h.t.Helper()
	for {
		changed := h.tg.expect(fmt.Sprintf("edit:%d", message))
		if strings.Contains(mustMessage(h, message).text, want) {
			return
		}
		h.wait(changed, want)
	}
}

func submitSelected(h *harness, message, count int) {
	h.t.Helper()
	calls := h.mp.holdCalls(downloadPath, count)
	h.tap(alice, message, downloadIt)
	for i, call := range calls {
		h.wait(call.entered, "the next batch submission")
		batchShows(h, message, fmt.Sprintf("正在提交 %d/%d", i, count))
		call.allow()
	}
	batchShows(h, message, "下载结果来啦")
}

// Each explicit confirmation starts one serial API batch. The UI remains
// responsive while the backend blocks and distinguishes mixed outcomes.
func TestBatchSubmissionShowsProgressAndMixedResults(t *testing.T) {
	h := pendingBatch(t)
	calls := h.mp.holdCalls(downloadPath, batchResources)
	confirm, _ := findButton(mustMessage(h, 1).rows, downloadIt)
	h.tap(alice, 1, downloadIt)
	h.wait(calls[0].entered, "the first API submission")
	h.shows(1, "正在提交 0/3")
	h.tapData(alice, 1, confirm)
	h.tap(bob, 1, stopBatch)
	h.say(alice, alice, "/help")
	h.tr.add(">> MoviePilot accepts the first resource")
	calls[0].allow()
	h.wait(calls[1].entered, "the second API submission")
	batchShows(h, 1, "正在提交 1/3")
	h.mp.setRoute(downloadPath, ok("download_rejected.json"))
	h.tr.add(">> MoviePilot refuses the second resource")
	calls[1].allow()
	h.wait(calls[2].entered, "the third API submission")
	batchShows(h, 1, "正在提交 2/3")
	h.mp.setRoute(downloadPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tr.add(">> MoviePilot leaves the third submission uncertain")
	failed := h.tg.failNextEdit(1)
	calls[2].allow()
	h.wait(failed, "the failed terminal batch edit")
	batchShows(h, 1, "下载结果来啦")
	h.shows(1, "任务添加失败")
	h.shows(1, "没能确认")
	h.shows(1, "开始下载 1 个资源")
	h.shows(1, "/tasks")
	if got := strings.Count(h.tr.String(), "-> MoviePilot "+downloadPath); got != batchResources {
		t.Errorf("batch submitted %d resources, want exactly %d", got, batchResources)
	}
	h.tap(alice, 1, "返回")
	h.shows(1, "帮你找到这些")
	h.neverShowsSiteCredentials()
	h.tr.verify(t)
}

// Stopping prevents subsequent API calls without cancelling an in-flight
// write or deleting the downloads it has already created.
func TestBatchStopKeepsCurrentSubmissionAndArrival(t *testing.T) {
	h := pendingBatch(t)
	calls := h.mp.holdCalls(downloadPath, batchResources)
	h.tap(alice, 1, downloadIt)
	h.wait(calls[0].entered, "the first API submission")
	h.tap(alice, 1, stopBatch)
	h.shows(1, "正在停止后续提交")
	if _, offered := findButton(mustMessage(h, 1).rows, "返回"); offered {
		t.Error("the batch offered navigation before its current submission settled")
	}
	h.tr.add(">> The current API submission succeeds after stop was requested")
	calls[0].allow()
	batchShows(h, 1, "下载结果来啦")
	h.shows(1, "未提交")
	h.shows(1, "开始下载 1 个资源")
	select {
	case <-calls[1].entered:
		t.Error("stopping the batch still sent its second resource")
	default:
	}
	file := mygo.file("S01", "E01")
	file.Hash = addedHash
	h.arrives(alice, file)
	h.shows(2, "MoviePilot 原生")
	h.tap(alice, 1, "返回")
	if strings.Contains(h.tr.String(), "DELETE ") || strings.Contains(h.tr.String(), "/download/stop/") {
		t.Error("stopping unsubmitted resources mutated an existing download")
	}
	h.tr.verify(t)
}

// A restart ends the worker before returning and cannot replay requests with
// an unknown outcome. Other users can use the newly started app normally.
func TestBatchRestartDoesNotReplaySubmissions(t *testing.T) {
	h := pendingBatch(t)
	calls := h.mp.holdCalls(downloadPath, batchResources)
	h.tap(alice, 1, downloadIt)
	h.wait(calls[0].entered, "an in-flight download API call")
	h.restart()
	h.wait(calls[0].exited, "the cancelled connection during shutdown")
	h.say(alice, alice, "/tasks")
	h.transfers()
	select {
	case <-calls[1].entered:
		t.Error("restart continued or replayed the old batch")
	default:
	}
	h.tap(alice, 1, stopBatch)
	h.shows(1, "/tasks")
	h.tr.verify(t)
}
