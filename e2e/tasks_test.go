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
// downloader; its owner can stop refreshing and go back to the list.
func TestFollowDownloadUntilItFinishes(t *testing.T) {
	h := start(t, scenario{routes: taskRoutes()})
	h.say(alice, alice, "/tasks")
	h.tap(alice, 1, mygoTask)
	h.shows(1, "进度 0%")
	h.reports(1, downloadsPath, "downloads_mygo_later.json")
	h.shows(1, "进度 43% · ↓ 3.1MB/s · 剩余 5分12秒")
	h.tap(alice, 1, stopRefresh)
	h.shows(1, "已停止自动刷新")
	h.tap(alice, 1, "返回任务列表")
	h.tap(alice, 1, mygoTask)
	h.reports(1, downloadsPath, "downloads_none.json")
	h.shows(1, "下载任务已结束")
	h.tr.verify(t)
}

// A followed transfer job shows each file's state until the queue empties;
// the finished view leads back to the remaining tasks.
func TestFollowTransferUntilItFinishes(t *testing.T) {
	h := start(t, scenario{routes: taskRoutes()})
	h.say(alice, alice, "/tasks")
	h.tap(alice, 1, mujicaTask)
	h.shows(1, "▶️ E13 整理中")
	h.reports(1, queuePath, "queue_mujica_later.json")
	h.shows(1, "已整理 1/2 个文件")
	h.reports(1, queuePath, "queue_none.json")
	h.shows(1, "整理任务已结束")
	h.tap(alice, 1, "返回任务列表")
	h.shows(1, "1. 《迷途之子!!!!!》第 1 季 · E10–E12 · 进度 0%")
	h.tr.verify(t)
}

// Without tasks /tasks just says so; opened from the home menu, it goes
// back to the menu.
func TestNoTasks(t *testing.T) {
	h := start(t, scenario{})
	h.say(alice, alice, "/tasks")
	h.shows(1, "当前没有下载中或整理中的任务")
	h.say(alice, alice, "/start")
	h.tap(alice, 2, "📋 任务进度")
	h.shows(2, "当前没有下载中或整理中的任务")
	h.tap(alice, 2, "📋 任务进度")
	h.shows(2, "当前没有下载中或整理中的任务")
	h.tr.verify(t)
}

// Buttons of a closed task list point back to /tasks, not /request.
func TestClosedTaskListCannotResume(t *testing.T) {
	h := start(t, scenario{routes: taskRoutes()})
	h.say(alice, alice, "/tasks")
	pick, _ := findButton(mustMessage(h, 1).rows, mygoTask)
	h.tap(alice, 1, "关闭")
	h.shows(1, "已关闭")
	h.tapData(alice, 1, pick)
	h.shows(1, "这个任务列表已失效，请重新 /tasks")
	h.tr.verify(t)
}
