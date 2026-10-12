package e2e

import (
	"net/http"
	"testing"
)

const (
	pauseIt      = "暂停下载"
	resumeIt     = "恢复下载"
	confirmPause = "确认暂停"
	viewTask     = "查看任务"
	mygoControl  = "PATCH /api/v1/download/5c1d0a2e8f3b4c6d7e8f9a0b1c2d3e4f5a6b7c8d"
)

// A download on a nondefault instance is paused and resumed there, each
// after its own confirmation. An accepted write shows the task live, in the
// state the downloader reports; a stale confirmation from before the write
// sends nothing.
func TestPauseAndResumeDownload(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{downloadsPath: ok(twoDownloaders(t))}})
	h.say(alice, alice, "/tasks")
	h.tap(alice, 1, "2")
	h.tap(alice, 1, pauseIt)
	h.shows(1, "要暂停这个下载吗")
	confirm, _ := findButton(mustMessage(h, 1).rows, confirmPause)
	h.tap(alice, 1, confirmPause)
	h.shows(1, "已提交暂停")
	h.shows(1, "已暂停 0%")
	h.tapData(alice, 1, confirm)
	h.tap(alice, 1, resumeIt)
	h.shows(1, "要恢复这个下载吗")
	h.tap(alice, 1, "确认恢复")
	h.shows(1, "已提交恢复")
	h.shows(1, "<h3>⬇️ 《迷途之子!!!!!》")
	h.tr.verify(t)
}

// A confirmation acts on the download as it is right before the write: one
// paused elsewhere meanwhile or gone is shown as it is, and one listed under
// two instances offers no pause, resume or deletion; none of them writes.
func TestDownloadControlChecksTargetFirst(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{downloadsPath: ok("downloads_mygo.json")}})
	h.say(alice, alice, "/tasks")
	h.tap(alice, 1, mygoTask)
	ask, _ := findButton(mustMessage(h, 1).rows, pauseIt)
	h.tap(alice, 1, pauseIt)
	h.mp.setRoute(downloadsPath, ok(downloadsWith(t, "downloads_mygo.json", func(data []map[string]any) {
		data[0]["state"] = "paused"
	})))
	h.tap(alice, 1, confirmPause)
	h.shows(1, "它已经是暂停状态啦")
	h.tap(alice, 1, resumeIt)
	h.mp.setRoute(downloadsPath, ok("downloads_none.json"))
	h.tap(alice, 1, "确认恢复")
	h.shows(1, "下载任务结束啦")

	h.mp.setRoute(downloadsPath, ok(downloadsWith(t, "downloads_dune_mygo.json", func(data []map[string]any) {
		data[0]["hash"], data[0]["downloader"] = data[1]["hash"], secondaryDownloader
	})))
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "2")
	for _, label := range []string{pauseIt, "删除任务"} {
		if _, shown := findButton(mustMessage(h, 1).rows, label); shown {
			t.Errorf("a task listed under two instances offers %s", label)
		}
	}
	h.tapData(alice, 1, ask)
	h.shows(1, "对应的下载器不唯一")
	h.tr.verify(t)
}

// A refusal is relayed. A lost response, or one that does not confirm the
// asked operation on this download, has an unknown result that leads only
// to a read of the task; nothing is sent twice.
func TestDownloadControlOutcomes(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{downloadsPath: ok(downloadsWith(t, "downloads_mygo.json", func(data []map[string]any) {
		data[0]["downloader"] = "qBittorrent-offline"
	}))}})
	h.say(alice, alice, "/tasks")
	h.tap(alice, 1, mygoTask)
	h.tap(alice, 1, pauseIt)
	h.tap(alice, 1, confirmPause)
	h.shows(1, "MoviePilot 没能暂停这个下载")

	h.mp.setRoute(downloadsPath, ok("downloads_mygo.json"))
	h.mp.setRoute(mygoControl, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, mygoTask)
	h.tap(alice, 1, pauseIt)
	h.tap(alice, 1, confirmPause)
	h.shows(1, "没能确认有没有暂停")

	h.mp.setRoute(mygoControl, ok("download_control_empty.json"))
	h.tap(alice, 1, viewTask)
	h.tap(alice, 1, pauseIt)
	h.tap(alice, 1, confirmPause)
	h.shows(1, "没能确认有没有暂停")
	h.tap(alice, 1, viewTask)
	h.shows(1, "<td>0%</td>")
	h.tr.verify(t)
}
