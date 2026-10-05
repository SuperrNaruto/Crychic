package flow

import (
	"context"
	"time"
)

const (
	// FollowFor bounds how long a task view refreshes itself before it
	// asks to be resumed, so forgotten views stop costing requests.
	FollowFor = 10 * time.Minute

	actionTask     = "k" // arg: index into session tasks; starts following
	actionFollow   = "f" // arg: index; one refresh of a followed task
	actionUnfollow = "u" // arg: index
	actionList     = "l" // back to the task list
	actionClose    = "c" // closes the task list

	msgTasksExpired = "⌛ 这个任务列表已失效，请重新 /tasks。"
	msgClosed       = "已关闭。"

	msgNotFollowing = "已停止刷新。"
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
	downloads, err := e.backend.Downloads(ctx)
	if err != nil {
		e.store.take(sess.id)
		return e.failure("downloads", err)
	}
	jobs, err := e.backend.Transfers(ctx)
	if err != nil {
		e.store.take(sess.id)
		return e.failure("transfers", err)
	}
	if len(downloads)+len(jobs) == 0 {
		e.store.take(sess.id)
		return Reply{Text: Sentence(msgNoTasks)}
	}
	sess.tasks, sess.follow = nil, following{}
	for _, d := range downloads {
		sess.tasks = append(sess.tasks, taskRef{download: true, id: d.ID})
	}
	for _, j := range jobs {
		sess.tasks = append(sess.tasks, taskRef{id: j.ID})
	}
	e.store.put(sess)
	return taskList(sess.id, downloads, jobs)
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
	case actionClose:
		e.store.take(sess.id)
		return Reply{Text: Sentence(msgClosed)}, true
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
		return e.failure("task", err)
	}
	if err != nil {
		e.log.Warn("task refresh failed", "task", sess.tasks[index].id, "err", err)
		view = withWarning(sess.follow.last, msgRefreshFailed)
	}
	if gone {
		sess.follow.on = false
		e.store.put(sess)
		view = ended(sess.follow.last, view)
		view.Buttons = [][]Button{{backButton(sess.id)}}
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
	live.Buttons = [][]Button{{{Label: "停止刷新", Data: data(sess.id, actionUnfollow, index)}, backButton(sess.id)}}
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
		return final
	}
	return Reply{Text: append(Lines(last.Text[0]), final.Text...), Image: last.Image}
}

// pausedView is the last view with refreshing stopped and a way to resume.
func pausedView(sess session, index int, why string) Reply {
	view := withWarning(sess.follow.last, why)
	view.Buttons = [][]Button{{{Label: "🔄 继续刷新", Data: data(sess.id, actionTask, index)}, backButton(sess.id)}}
	return view
}

// withWarning appends a note to a view without touching the original.
func withWarning(view Reply, note string) Reply {
	text := append(append(Text{}, view.Text...), Line(Emphasis(note)))
	return Reply{Text: text, Image: view.Image}
}

func backButton(id uint64) Button {
	return Button{Label: "返回任务列表", Data: data(id, actionList, 0)}
}

// expired tells the user how to start over: buttons of a task list lead
// back to /tasks, all others to /request.
func expired(action string) Reply {
	switch action {
	case actionTask, actionFollow, actionUnfollow, actionList, actionClose:
		return Reply{Text: Sentence(msgTasksExpired)}
	case actionMedia, actionSeason, actionConfirm, actionAskFrom, actionCancel, actionMulti, actionTick, actionSubscribed:
		return Reply{Text: Sentence(msgExpired)}
	}
	return Reply{Text: Sentence(msgHomeExpired)}
}
