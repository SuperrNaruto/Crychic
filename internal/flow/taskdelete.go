package flow

import "context"

const (
	actionAskDelete = "kd" // arg: index into session tasks; asks to delete that download
	actionDelete    = "ky" // arg: index; deletes it

	msgDeleteTitle   = "删除下载任务"
	msgDeleteAsk     = "⚠️ 要删除这个下载任务吗？"
	msgDeleteFiles   = "会把已经下载的文件一起删掉，删了就找不回来啦。"
	msgDeleteSub     = "它还在订阅里：如果这个任务是订阅下的，MoviePilot 已经把它记成下好了，删掉后订阅不会自动重新下载哦。"
	msgDeleteUnknown = "呜，没能确认有没有删掉…用 /tasks 看一眼吧。"
	msgDeleteLabel   = "确认删除"
)

// deleteButton offers to delete the followed download index.
func deleteButton(id uint64, index int) Button {
	return Button{Label: "删除任务", Data: data(id, actionAskDelete, index)}
}

// chooseDelete applies the download deletion steps; ok is false for others.
func (e *Engine) chooseDelete(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionAskDelete:
		return e.askDelete(ctx, sess, p.arg), true
	case actionDelete:
		return e.deleteDownload(ctx, sess, p.arg), true
	}
	return Reply{}, false
}

// askDelete stops following download index and asks before deleting it,
// warning when a subscription of the same media and season exists (the
// downloader does not say which subscription, if any, added it). That check is a courtesy, so
// a failed one still asks.
func (e *Engine) askDelete(ctx context.Context, sess session, index int) Reply {
	if !sess.isDownload(index) {
		return Reply{Notice: msgInvalidChoice}
	}
	sess.follow.on, sess.doomed = false, nil
	downloads, err := e.backend.Downloads(ctx)
	if err != nil {
		e.store.put(sess)
		return withTaskButtons(titled(msgTasksTitle, e.failure("downloads", err)), sess)
	}
	d, found := findDownload(downloads, sess.tasks[index].downloadRef())
	if !found {
		e.store.put(sess)
		return withTaskButtons(titled(msgTasksTitle, Reply{Text: Sentence(msgDownloadGone)}), sess)
	}
	sess.doomed = &d.DownloadRef
	e.store.put(sess)
	view := downloadView(d)
	view.Text = append(view.Text, Divider(), Line(Strong(msgDeleteAsk)), Line(Plain(msgDeleteFiles)))
	if e.subscribed(ctx, d) {
		view.Text = append(view.Text, Line(Plain(msgDeleteSub)))
	}
	view.Buttons = [][]Button{{{Label: msgDeleteLabel, Data: data(sess.id, actionDelete, index)}}, {tasksBack(sess.id)}}
	return view
}

// subscribed reports whether a subscription of the backend is for d.
func (e *Engine) subscribed(ctx context.Context, d Download) bool {
	subs, err := e.backend.Subscriptions(ctx)
	if err != nil {
		e.log.Warn("subscriptions unavailable", "download", d.ID, "err", err)
		return false
	}
	for _, s := range subs {
		if s.Source == d.Source && s.MediaID == d.MediaID && s.Kind == d.Kind && seasonOf(s.Season) == seasonOf(d.Season) {
			return true
		}
	}
	return false
}

// deleteDownload deletes download index once. Steps of a session run one
// at a time and the download to confirm is cleared before the write, so a
// second tap only flashes a notice; the session stays for 返回 to the list.
func (e *Engine) deleteDownload(ctx context.Context, sess session, index int) Reply {
	if !sess.isDownload(index) || sess.doomed == nil || *sess.doomed != sess.tasks[index].downloadRef() {
		return Reply{Notice: msgInvalidChoice}
	}
	ref := *sess.doomed
	sess.doomed = nil
	e.store.put(sess)
	if err := e.backend.DeleteDownload(ctx, ref); err != nil {
		return withTaskButtons(titled(msgDeleteTitle, e.deleteFailure(ref.ID, err)), sess)
	}
	if err := e.watcher.ForgetDownload(ctx, ref.ID); err != nil {
		e.log.Error("cannot forget deleted download", "download", ref.ID, "err", err)
	}
	done := Lines(Heading(Plain("✅ 已删除")), Line(Plain("已经删掉啦，下载的文件也一起清掉了～")))
	return withTaskButtons(Reply{Text: done}, sess)
}

// deleteFailure passes a MoviePilot refusal on; anything else may still
// have deleted the download.
func (e *Engine) deleteFailure(id string, err error) Reply {
	if _, safe := UserMessage(err); safe {
		return e.failure("delete download", err)
	}
	e.log.Error("download deletion result unknown", "download", id, "err", err)
	return Reply{Text: Sentence(msgDeleteUnknown)}
}

// isDownload reports whether task index of the session is a download.
func (sess session) isDownload(index int) bool {
	return index >= 0 && index < len(sess.tasks) && sess.tasks[index].download
}

func findDownload(downloads []Download, ref DownloadRef) (Download, bool) {
	for _, d := range downloads {
		if d.DownloadRef == ref {
			return d, true
		}
	}
	return Download{}, false
}

// withTaskButtons leads back to the task list.
func withTaskButtons(reply Reply, sess session) Reply {
	reply.Buttons = taskRows(sess, nil)
	return reply
}
