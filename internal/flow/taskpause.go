package flow

import "context"

const (
	actionAskPause  = "kp" // arg: index into session tasks; asks to pause that download
	actionAskResume = "kr" // arg: index; asks to resume it
	actionPause     = "kq" // arg: index; pauses it once (ticketed)
	actionResume    = "ks" // arg: index; resumes it once (ticketed)

	labelViewTask       = "查看任务"
	msgControlElsewhere = "这个任务在 MoviePilot 里对应的下载器不唯一，暂停、恢复和删除请到 MoviePilot 里操作哦～"
)

// downloadControl is what a download's last read lets its view offer.
type downloadControl uint8

const (
	noControl downloadControl = iota // a transfer job, an unread task or an ambiguous instance
	canPause
	canResume
)

// downloadToggle is the download, on its own instance, asked to be paused
// (pause) or resumed.
type downloadToggle struct {
	DownloadRef
	pause bool
}

// toggleWords are one direction's copy.
type toggleWords struct {
	title, button, ask, keep, confirm, done, already, unknown string
}

var pauseWords = toggleWords{
	title: "📋 暂停下载", button: "暂停下载", confirm: "确认暂停", done: "已提交暂停，下载器的状态会自动刷新到这里～",
	ask:     "⏸️ 要暂停这个下载吗？",
	keep:    "已经下载的部分都留着，之后点「恢复下载」就能接着下～",
	already: "它已经是暂停状态啦～",
	unknown: "呜，没能确认有没有暂停…点「查看任务」看看实际状态吧～",
}

var resumeWords = toggleWords{
	title: "📋 恢复下载", button: "恢复下载", confirm: "确认恢复", done: "已提交恢复，下载器的状态会自动刷新到这里～",
	ask:     "⬇️ 要恢复这个下载吗？",
	keep:    "下载器会从暂停的地方接着下～",
	already: "它已经在下载啦～",
	unknown: "呜，没能确认有没有恢复…点「查看任务」看看实际状态吧～",
}

func wordsFor(pause bool) toggleWords {
	if pause {
		return pauseWords
	}
	return resumeWords
}

func confirmAction(pause bool) string {
	if pause {
		return actionPause
	}
	return actionResume
}

// controlOf offers the opposite of d's state; a task without one known
// instance gets no write control, so no request falls back to the default.
func controlOf(d Download) downloadControl {
	switch {
	case d.InstanceAmbiguous || d.Downloader == "":
		return noControl
	case d.Paused:
		return canResume
	}
	return canPause
}

// toggleButton asks to pause or resume download index.
func toggleButton(id uint64, index int, control downloadControl) Button {
	if control == canResume {
		return Button{Label: resumeWords.button, Data: data(id, actionAskResume, index)}
	}
	return Button{Label: pauseWords.button, Data: data(id, actionAskPause, index)}
}

// chooseToggle applies the pause and resume steps; ok is false for others.
func (e *Engine) chooseToggle(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionAskPause, actionAskResume:
		return e.askToggle(ctx, sess, p.arg, p.action == actionAskPause), true
	case actionPause, actionResume:
		return e.toggleDownload(ctx, sess, p.arg, p.action == actionPause), true
	}
	return Reply{}, false
}

// askToggle stops following download index, reads it afresh and asks
// before pausing or resuming it.
func (e *Engine) askToggle(ctx context.Context, sess session, index int, pause bool) Reply {
	if !sess.isDownload(index) {
		return Reply{Notice: msgInvalidChoice}
	}
	sess.follow.on, sess.doomed, sess.toggle = false, nil, nil
	d, instead, ok := e.controllable(ctx, &sess, index, pause)
	if ok {
		sess.toggle = &downloadToggle{DownloadRef: d.DownloadRef, pause: pause}
	}
	e.store.put(sess)
	if !ok {
		return instead
	}
	words := wordsFor(pause)
	view := downloadView(d)
	view.Text = append(view.Text, Divider(), Line(Strong(words.ask)), Line(Plain(words.keep)))
	view.Buttons = [][]Button{{{Label: words.confirm, Data: data(sess.id, confirmAction(pause), index)}}, {tasksBack(sess.id)}}
	return view
}

// toggleDownload pauses or resumes download index once. The asked target is
// consumed before anything else, so a second tap only flashes a notice; the
// download is read again right before the write, because the downloader
// accepts a hash it no longer holds. An accepted write shows the task live.
func (e *Engine) toggleDownload(ctx context.Context, sess session, index int, pause bool) Reply {
	asked := sess.toggle
	if !sess.isDownload(index) || asked == nil || asked.pause != pause || asked.DownloadRef != sess.tasks[index].downloadRef() {
		return Reply{Notice: msgInvalidChoice}
	}
	sess.toggle = nil
	e.store.put(sess)
	_, instead, ok := e.controllable(ctx, &sess, index, pause)
	e.store.put(sess)
	if !ok {
		return instead
	}
	words := wordsFor(pause)
	check := []Button{{Label: labelViewTask, Data: data(sess.id, actionTask, index)}}
	if err := e.backend.SetDownloadPaused(ctx, asked.DownloadRef, pause); err != nil {
		reply := titled(words.title, e.toggleFailure(asked.ID, words, err))
		reply.Buttons = taskRows(sess, check)
		return reply
	}
	sess.follow = following{on: true, until: e.now().Add(FollowFor)}
	live := e.refresh(ctx, sess, index)
	live.Text = append(live.Text, Remark(words.done))
	return live
}

// controllable reads download index afresh and returns it when it can be
// paused (pause) or resumed now; otherwise ok is false and instead says
// why, without any write.
func (e *Engine) controllable(ctx context.Context, sess *session, index int, pause bool) (d Download, instead Reply, ok bool) {
	downloads, err := e.backend.Downloads(ctx)
	if err != nil {
		return Download{}, withTaskButtons(titled(msgTasksTitle, e.failure("downloads", err)), *sess), false
	}
	d, found := findDownload(downloads, sess.tasks[index].downloadRef())
	if !found {
		return Download{}, withTaskButtons(titled(msgTasksTitle, Reply{Text: Sentence(msgDownloadGone)}), *sess), false
	}
	sess.follow.last, sess.follow.control = downloadView(d), controlOf(d)
	switch {
	case sess.follow.control == noControl:
		return Download{}, pausedView(*sess, index, msgControlElsewhere), false
	case d.Paused == pause:
		return Download{}, pausedView(*sess, index, wordsFor(pause).already), false
	}
	return d, Reply{}, true
}

// toggleFailure passes a MoviePilot refusal on; anything else may still
// have changed the download, so it points to a read.
func (e *Engine) toggleFailure(id string, words toggleWords, err error) Reply {
	if _, safe := UserMessage(err); safe {
		return e.failure("toggle download", err)
	}
	e.log.Error("download control result unknown", "download", id, "err", err)
	return Reply{Text: Sentence(words.unknown)}
}
