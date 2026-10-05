package e2e

import "testing"

const (
	mygoTask    = "1. 迷途之子!!!!! 第 1 季"
	mujicaTask  = "2. 颂乐人偶 第 1 季"
	stopRefresh = "停止刷新"
)

func taskRoutes() map[string]route {
	return map[string]route{
		downloadsPath: ok("downloads_mygo.json"),
		queuePath:     ok("queue_mujica.json"),
	}
}

// A followed download keeps its message current until it leaves the
// downloader; stopping and resuming refresh is up to its owner.
func TestFollowDownloadUntilItFinishes(t *testing.T) {
	h := start(t, scenario{routes: taskRoutes()})
	h.say(alice, alice, "/tasks")
	h.tap(alice, 1, mygoTask)
	h.shows(1, "进度 0%")
	h.reports(1, downloadsPath, "downloads_mygo_later.json")
	h.shows(1, "进度 43% · ↓ 3.1MB/s · 剩余 5分12秒")
	h.tap(alice, 1, stopRefresh)
	h.shows(1, "已停止自动刷新")
	h.tap(alice, 1, "🔄 继续刷新")
	h.reports(1, downloadsPath, "downloads_none.json")
	h.shows(1, "下载任务已结束")
	h.tr.verify(t)
}

// A followed transfer job shows each file's state until the queue empties.
func TestFollowTransferUntilItFinishes(t *testing.T) {
	h := start(t, scenario{routes: taskRoutes()})
	h.say(alice, alice, "/tasks")
	h.tap(alice, 1, mujicaTask)
	h.shows(1, "▶️ E13 整理中")
	h.reports(1, queuePath, "queue_mujica_later.json")
	h.shows(1, "已整理 1/2 个文件")
	h.reports(1, queuePath, "queue_none.json")
	h.shows(1, "整理任务已结束")
	h.tr.verify(t)
}

func TestNoTasks(t *testing.T) {
	h := start(t, scenario{})
	h.say(alice, alice, "/tasks")
	h.shows(1, "当前没有下载中或整理中的任务")
	h.tr.verify(t)
}
