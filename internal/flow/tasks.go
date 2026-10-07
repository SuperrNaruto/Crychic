package flow

import (
	"context"
	"strings"
	"sync"
	"time"
)

const (
	// FollowFor bounds how long a task view refreshes itself before it
	// asks to be resumed, so forgotten views stop costing requests.
	FollowFor = 10 * time.Minute

	actionTask     = "k" // arg: index into session tasks; starts following
	actionFollow   = "f" // arg: index; one refresh of a followed task
	actionUnfollow = "u" // arg: index
	actionList     = "l" // back to the task list, read afresh

	msgTasksExpired = "⌛ 这个任务列表过期啦，重新 /tasks 一下吧～"

	msgNotFollowing = "已经不刷新啦～"
)

// taskRef identifies a listed task across refreshes.
type taskRef struct {
	download bool
	id       string
}

// following is the live view of one task in a session.
type following struct {
	on    bool
	until time.Time
	last  Reply // shown again, with a warning, when a refresh fails
}

// Tasks lists the backend's downloads and transfer jobs to pick one to follow.
func (e *Engine) Tasks(ctx context.Context, actor Actor) Reply {
	return e.listTasks(ctx, e.store.create(actor, nil))
}

// listTasks shows the current tasks in sess, which stops following any.
func (e *Engine) listTasks(ctx context.Context, sess session) Reply {
	downloads, jobs, err := e.taskLists(ctx)
	if err != nil {
		e.store.take(sess.id)
		return titled(msgTasksTitle, e.failure("transfers", err))
	}
	if len(downloads)+len(jobs) == 0 && sess.menu {
		return Reply{Notice: msgNoTasks}
	}
	if len(downloads)+len(jobs) == 0 {
		e.store.take(sess.id)
		return Reply{Text: Lines(Heading(Plain(msgTasksTitle)), Line(Plain(msgNoTasks)))}
	}
	sess.tasks, sess.follow = nil, following{}
	for _, d := range downloads {
		sess.tasks = append(sess.tasks, taskRef{download: true, id: d.ID})
	}
	for _, j := range jobs {
		sess.tasks = append(sess.tasks, taskRef{id: j.ID})
	}
	e.store.put(sess)
	return e.listPages(sess, taskList(sess.id, downloads, jobs))
}

func (e *Engine) taskLists(ctx context.Context) ([]Download, []TransferJob, error) {
	var downloads []Download
	var jobs []TransferJob
	var downloadErr, jobErr error
	var wg sync.WaitGroup
	wg.Go(func() { downloads, downloadErr = e.backend.Downloads(ctx) })
	wg.Go(func() { jobs, jobErr = e.backend.Transfers(ctx) })
	wg.Wait()
	if downloadErr != nil {
		return nil, nil, downloadErr
	}
	return downloads, jobs, jobErr
}

// chooseTask applies the task actions; ok is false for any other action.
func (e *Engine) chooseTask(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionTask:
		sess.follow = following{on: true, until: e.now().Add(FollowFor)}
		return e.refresh(ctx, sess, p.arg), true
	case actionFollow:
		if !sess.follow.on {
			return Reply{Notice: msgNotFollowing}, true
		}
		return e.refresh(ctx, sess, p.arg), true
	case actionUnfollow:
		sess.follow.on = false
		e.store.put(sess)
		return pausedView(sess, p.arg, msgUnfollowed), true
	case actionList:
		return e.listTasks(ctx, sess), true
	}
	return Reply{}, false
}

// refresh shows task index as it is now and keeps following it until it
// leaves the backend's lists or FollowFor runs out.
func (e *Engine) refresh(ctx context.Context, sess session, index int) Reply {
	if index < 0 || index >= len(sess.tasks) {
		return Reply{Notice: msgInvalidChoice}
	}
	view, gone, err := e.taskView(ctx, sess.tasks[index])
	if err != nil && len(sess.follow.last.Text) == 0 {
		e.store.take(sess.id)
		return titled(msgTasksTitle, e.failure("task", err))
	}
	if err != nil {
		e.log.Warn("task refresh failed", "task", sess.tasks[index].id, "err", err)
		view = withWarning(sess.follow.last, msgRefreshFailed)
	}
	if gone {
		sess.follow.on = false
		e.store.put(sess)
		view = ended(sess.follow.last, view)
		view.Buttons = taskRows(sess, nil)
		return view
	}
	if err == nil {
		sess.follow.last = view
	}
	if e.now().After(sess.follow.until) {
		sess.follow.on = false
		e.store.put(sess)
		return pausedView(sess, index, msgFollowExpired)
	}
	e.store.put(sess)
	live := withWarning(view, msgFollowing)
	live.Buttons = taskRows(sess, sess.taskControls(index, Button{Label: "停止刷新", Data: data(sess.id, actionUnfollow, index)}))
	live.Follow = data(sess.id, actionFollow, index)
	return live
}

// taskView renders the current state of ref; gone means it has finished
// and the view is final.
func (e *Engine) taskView(ctx context.Context, ref taskRef) (view Reply, gone bool, err error) {
	if ref.download {
		downloads, err := e.backend.Downloads(ctx)
		if err != nil {
			return Reply{}, false, err
		}
		for _, d := range downloads {
			if d.ID == ref.id {
				return downloadView(d), false, nil
			}
		}
		return Reply{Text: Sentence(msgDownloadGone)}, true, nil
	}
	jobs, err := e.backend.Transfers(ctx)
	if err != nil {
		return Reply{}, false, err
	}
	for _, j := range jobs {
		if j.ID == ref.id {
			return transferView(j), false, nil
		}
	}
	return Reply{Text: Sentence(msgTransferGone)}, true, nil
}

// ended keeps the title line and poster of the last view above the final
// word, so the message still says which task finished.
func ended(last, final Reply) Reply {
	if len(last.Text) == 0 {
		return titled(msgTasksTitle, final)
	}
	return Reply{Text: append(Lines(last.Text[0]), final.Text...), Image: last.Image}
}

// pausedView is the last view with refreshing stopped and a way to resume.
func pausedView(sess session, index int, why string) Reply {
	view := withWarning(sess.follow.last, why)
	view.Buttons = taskRows(sess, sess.taskControls(index, Button{Label: "继续刷新", Data: data(sess.id, actionTask, index)}))
	return view
}

// withWarning appends a note to a view without touching the original.
func withWarning(view Reply, note string) Reply {
	text := append(append(Text{}, view.Text...), Remark(note))
	return Reply{Text: text, Image: view.Image}
}

// taskControls are refresh control's row in task index's view, with
// 删除任务 for a download.
func (sess session) taskControls(index int, refresh Button) []Button {
	if sess.isDownload(index) {
		return []Button{refresh, deleteButton(sess.id, index)}
	}
	return []Button{refresh}
}

// taskRows are a task view's buttons: its controls above 返回 to the list
// read afresh, 首页 and 关闭.
func taskRows(sess session, controls []Button) [][]Button {
	nav := append([]Button{tasksBack(sess.id)}, browseRow(sess.id)...)
	if len(controls) == 0 {
		return [][]Button{nav}
	}
	return [][]Button{controls, nav}
}

// tasksBack leads back to the task list, read afresh.
func tasksBack(id uint64) Button {
	return backTo(id, actionList, 0)
}

// expired tells the user how to start over: buttons of a task list lead
// back to /tasks, all others to /search.
func expired(action string) Reply {
	switch action {
	case actionSubs, actionSubsKind, actionCancelPick, actionHistory, actionHistoryKind,
		actionHistoryPick, actionResubscribe, actionSubDetail, actionRefreshSubDetail:
		return expiredText("⌛ 这个订阅列表过期啦，重新 /subscribe 一下吧～")
	case actionTask, actionFollow, actionUnfollow, actionList, actionAskDelete, actionDelete:
		return expiredText(msgTasksExpired)
	case actionMedia, actionSeason, actionConfirm, actionAskFrom, actionCancel, actionMulti, actionTick, actionSubscribed,
		actionRelated, actionSeries, actionBack, actionResearch, actionRetry,
		actionTorrents, actionTorrentRun, actionTorrentRetry, actionTorrentAgain, actionTorrentPick, actionTorrentGet,
		actionTorrentSort, actionTorrentSites, actionTorrentSite:
		return expiredText(msgExpired)
	}
	return expiredText(msgHomeExpired)
}

// expiredText is an expiry message under its heading, e.g. "⌛ 过期啦"
// over "这个请求过期啦，重新 /search 一下吧～".
func expiredText(message string) Reply {
	return Reply{Text: Lines(Heading(Plain("⌛ 过期啦")), Line(Plain(strings.TrimPrefix(message, "⌛ "))))}
}
