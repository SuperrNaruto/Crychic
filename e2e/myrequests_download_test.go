package e2e

import (
	"maps"
	"strings"
	"testing"
)

func TestMyRequestsDownloadIdentity(t *testing.T) {
	h := start(t, scenario{routes: movieRequestRoutes()})
	requestMovieDownload(h)
	mine := requestMovieTask()
	other := maps.Clone(mine)
	other["hash"], other["progress"] = "unrelated-hash", 99
	h.mp.setRoute(downloadsPath, ok(writeFixture(t, []map[string]any{mine, other})))
	h.mp.setRoute(queuePath, ok(writeFixture(t, requestMovieQueue())))
	before := h.tr.String()
	h.say(alice, alice, "/requests")
	h.tap(alice, 2, "1")
	h.shows(2, "下载中 37%")
	h.shows(2, "整理状态未知")
	if strings.Contains(mustMessage(h, 2).text, "整理失败") {
		t.Error("another download supplied this request's failed transfer")
	}
	checkRequestQueueInstances(h, mine)
	other["hash"], other["downloader"] = addedHash, "another-"+requestPrivate
	h.mp.setRoute(downloadsPath, ok(writeFixture(t, []map[string]any{mine, other})))
	h.tap(alice, 2, "刷新")
	h.shows(2, "下载实例无法确定")
	h.shows(2, "<td>未知</td>")
	if strings.Contains(mustMessage(h, 2).text, "99%") || strings.Contains(mustMessage(h, 2).text, "37%") {
		t.Error("ambiguous download instances were attributed to one request")
	}
	assertRequestReadsOnly(h, before)
	assertRequestPrivate(h)
	h.neverShowsSiteCredentials()
	h.tr.verify(t)
}

func checkRequestQueueInstances(h *harness, mine map[string]any) {
	h.t.Helper()
	queue := requestMovieQueue()
	files := queue[0]["tasks"].([]map[string]any)
	files[1]["download_hash"], files[1]["downloader"] = addedHash, "another-"+requestPrivate
	h.mp.setRoute(queuePath, ok(writeFixture(h.t, queue)))
	h.tap(alice, 2, "刷新")
	h.shows(2, "下载实例无法确定")
	h.shows(2, "<td>未知</td>")
	h.mp.setRoute(queuePath, ok(writeFixture(h.t, requestMovieQueue())))
	unknown := map[string]any{"hash": addedHash, "downloader": "another-" + requestPrivate, "progress": 99}
	h.mp.setRoute(downloadsPath, ok(writeFixture(h.t, []map[string]any{mine, unknown})))
	h.tap(alice, 2, "刷新")
	h.shows(2, "下载实例无法确定")
	file := duneFile()
	file.ID, file.Hash, file.FailureStage = 1, addedHash, "source_access"
	h.mp.setRoute(failedTransfersPath, ok(writeFixture(h.t, map[string]any{"list": []transfer{file}, "total": 1})))
	h.mp.setRoute(downloadsPath, ok(writeFixture(h.t, []map[string]any{mine})))
	h.tap(alice, 2, "刷新")
	h.shows(2, "失败历史缺少下载器实例")
	if strings.Contains(mustMessage(h, 2).text, "源文件访问") {
		h.t.Error("hash-only failure history claimed a downloader instance")
	}
}

func movieRequestRoutes() map[string]route {
	return map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"),
		duneLookup: ok("subscription_none.json"), duneTorrents: searchingSites("torrents_dune.json"),
		downloadPath: ok("download_added.json"), subsPath: ok("subscriptions_none.json"),
	}
}

func requestMovieDownload(h *harness) {
	h.t.Helper()
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.searchTorrents(alice, 1, searchResources)
	h.tap(alice, 1, "1")
	h.tap(alice, 1, downloadIt)
}

func requestMovieTask() map[string]any {
	return map[string]any{
		"hash": addedHash, "downloader": requestPrivate, "progress": 37, "state": "downloading",
		"media": map[string]any{"media_source": "themoviedb", "media_id": "438631", "type": "电影", "title": requestPrivate},
	}
}

func requestMovieQueue() []map[string]any {
	return []map[string]any{{
		"media": map[string]any{"media_source": "themoviedb", "media_id": "438631", "type": "电影", "title": requestPrivate},
		"tasks": []map[string]any{
			{"state": requestPrivate, "download_hash": addedHash, "downloader": requestPrivate, "fileitem": map[string]any{"path": "/private/" + requestPrivate}},
			{"state": "failed", "download_hash": "unrelated-hash", "downloader": requestPrivate},
		},
	}}
}
