package e2e

import "testing"

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
	h.tap(alice, 1, "返回任务列表")
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
	h.tap(alice, 1, "返回任务列表")
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
	h.tap(alice, 2, "订阅")
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
