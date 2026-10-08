package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	mygoTask    = "1"
	mujicaTask  = "2"
	stopRefresh = "停止刷新"
)

func taskRoutes() map[string]route {
	return map[string]route{
		downloadsPath: ok("downloads_mygo.json"),
		queuePath:     ok("queue_mujica.json"),
	}
}

// A followed download keeps its message current until it leaves the
// downloader; its owner can stop refreshing and go back to the list.
func TestFollowDownloadUntilItFinishes(t *testing.T) {
	h := start(t, scenario{routes: taskRoutes()})
	h.say(alice, alice, "/start")
	h.tap(alice, 1, "任务进度")
	h.tap(alice, 1, mygoTask)
	h.shows(1, "<td>0%</td>")
	h.reports(1, downloadsPath, "downloads_mygo_later.json")
	h.shows(1, "<td>43%</td><td>3.1MB/s</td><td>5分12秒</td>")
	h.tap(alice, 1, stopRefresh)
	h.shows(1, "不自动刷新啦")
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, mygoTask)
	h.reports(1, downloadsPath, "downloads_none.json")
	h.shows(1, "下载任务结束啦")
	h.tr.verify(t)
}

// A followed transfer job shows each file's state until the queue empties;
// the finished view leads back to the remaining tasks.
func TestFollowTransferUntilItFinishes(t *testing.T) {
	h := start(t, scenario{routes: taskRoutes()})
	h.say(alice, alice, "/tasks")
	h.tap(alice, 1, mujicaTask)
	h.shows(1, "<td>E13</td><td>▶️ 整理中</td>")
	h.reports(1, queuePath, "queue_mujica_later.json")
	h.shows(1, "已整理 1/2 个文件")
	h.reports(1, queuePath, "queue_none.json")
	h.shows(1, "整理任务结束啦")
	h.tap(alice, 1, "返回")
	h.shows(1, "<i>E10–E12 · 17.5 GiB · 进度 0%</i>")
	h.tr.verify(t)
}

// With nothing to list, /tasks says so in a message; a home menu button
// only flashes the same words and the menu stays.
func TestNothingToList(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{subsPath: ok("subscriptions_none.json")}})
	h.say(alice, alice, "/tasks")
	h.shows(1, "现在没有在下载或整理的任务")
	h.say(alice, alice, "/start")
	h.tap(alice, 2, "任务进度")
	h.tap(alice, 2, "我的订阅")
	h.shows(2, "我帮你找片")
	h.tr.verify(t)
}

// Buttons of a closed task list point back to /tasks, not /search.
func TestClosedTaskListCannotResume(t *testing.T) {
	h := start(t, scenario{routes: taskRoutes()})
	h.say(alice, alice, "/tasks")
	pick, _ := findButton(mustMessage(h, 1).rows, mygoTask)
	h.tap(alice, 1, "关闭")
	h.shows(1, "已经关掉啦")
	h.tapData(alice, 1, pick)
	h.shows(1, "这个任务列表过期啦，重新 /tasks")
	h.tr.verify(t)
}

// A download picked by hand can be deleted from its task view after a
// warning about its files (and, when a subscription has it, that the
// subscription will not fetch it again); once deleted it leaves the list,
// its arrival is no longer announced and a second tap does nothing. A
// deletion with an unknown result points back to /tasks.
func TestDeleteDownloadTask(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:   ok("search_dune.json"),
		duneDetails:  ok("detail_dune.json"),
		duneLookup:   ok("subscription_none.json"),
		duneTorrents: searchingSites("torrents_dune.json"),
		downloadPath: ok("download_added.json"),
		subsPath:     ok("subscriptions.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.searchTorrents(alice, 1, searchResources)
	h.tap(alice, 1, "2")
	h.tap(alice, 1, downloadIt)

	h.mp.setRoute(downloadsPath, ok("downloads_dune_mygo.json"))
	h.say(alice, alice, "/tasks")
	h.tap(alice, 2, "1")
	h.tap(alice, 2, "删除任务")
	h.shows(2, "会把已经下载的文件一起删掉")
	h.shows(2, "如果这个任务是订阅下的")
	confirm, _ := findButton(mustMessage(h, 2).rows, "确认删除")
	h.tap(alice, 2, "确认删除")
	h.shows(2, "已经删掉啦")
	h.tapData(alice, 2, confirm)
	file := duneFile()
	file.Hash = addedHash
	h.transfers(file)

	h.tap(alice, 2, "返回")
	h.tap(alice, 2, "1")
	h.tap(alice, 2, "删除任务")
	h.mp.setRoute("DELETE /api/v1/download/5c1d0a2e8f3b4c6d7e8f9a0b1c2d3e4f5a6b7c8d",
		route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, 2, "确认删除")
	h.shows(2, "没能确认有没有删掉")
	h.tr.verify(t)
}

// A task from a nondefault downloader is deleted from that instance, not
// from the default one, whose response succeeds even for an absent hash.
func TestDeleteFromNondefaultDownloader(t *testing.T) {
	const title = "迷途之子!!!!!"
	h := start(t, scenario{routes: map[string]route{
		downloadsPath: ok(twoDownloaders(t)),
		subsPath:      ok("subscriptions_none.json"),
	}})
	h.say(alice, alice, "/tasks")
	h.tap(alice, 1, "2")
	h.shows(1, title)
	h.tap(alice, 1, "删除任务")
	h.tap(alice, 1, "确认删除")
	h.shows(1, "已经删掉啦")
	h.tap(alice, 1, "返回")
	h.shows(1, "《沙丘》")
	if strings.Contains(mustMessage(h, 1).text, title) {
		t.Error("the nondefault downloader's task remains after a successful deletion")
	}
	h.tr.verify(t)
}

func twoDownloaders(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "moviepilot", "downloads_dune_mygo.json"))
	if err != nil {
		t.Fatal(err)
	}
	var env struct{ Data []map[string]any }
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	env.Data[1]["downloader"] = "qBittorrent-secondary"
	return writeFixture(t, env.Data)
}
