package flow

import "fmt"

const (
	msgNoTasks       = "当前没有下载中或整理中的任务。"
	msgUnfollowed    = "已停止自动刷新。"
	msgRefreshFailed = "⚠️ 暂时拿不到最新进度，稍后自动重试。"
	msgFollowing     = "自动刷新中…"
	msgDownloadGone  = "下载任务已结束（下载完成或被移除）。"
	msgTransferGone  = "整理任务已结束，入库情况见入库通知或 MoviePilot 的整理历史。"
	// idleSpeed is how MoviePilot words a stalled download's speed.
	idleSpeed        = "0.0B"
	maxTransferLines = 100
)

var msgFollowExpired = fmt.Sprintf("已自动刷新 %d 分钟，暂停刷新。", int(FollowFor.Minutes()))

// fileStateText words each transfer file state with its marker.
var fileStateText = map[FileState]string{
	FileWaiting: "⏳ %s 等待中",
	FileRunning: "▶️ %s 整理中",
	FileDone:    "✅ %s 已完成",
	FileFailed:  "⚠️ %s 失败",
}

// taskList numbers downloads, then transfer jobs, matching session tasks.
func taskList(id uint64, downloads []Download, jobs []TransferJob) listView {
	view := listView{heading: Line(Strong("📋 进行中的任务")), footer: []Button{{Label: "关闭", Data: data(id, actionClose, 0)}}}
	section := "⬇️ 下载"
	add := func(title string, season *int, line string) {
		n := len(view.entries) + 1
		title = truncate(title, listTitleRunes)
		text := entry(n, Strong(taskName(title, season)), truncate(line, listTitleRunes))
		if section != "" {
			text = append(Lines(Line(Strong(section))), text...)
			section = ""
		}
		view.entries = append(view.entries, listEntry{text: text, buttons: []Button{{Label: fmt.Sprint(n), Data: data(id, actionTask, n-1)}}})
	}
	for _, d := range downloads {
		add(d.Title, d.Season, joinNonEmpty(" · ", EpisodeRanges(d.Episodes), progress(d)))
	}
	section = "📦 整理"
	for _, j := range jobs {
		add(j.Title, j.Season, filesDone(j))
	}
	return view
}

// downloadView is one download, e.g. "进度 37% · ↓ 3.1MB/s · 剩余 5分12秒".
func downloadView(d Download) Reply {
	icon := "⬇️ "
	if d.Paused {
		icon = "⏸️ "
	}
	head := joinNonEmpty(" ", taskName(d.Title, d.Season), EpisodeRanges(d.Episodes))
	speed := ""
	if !d.Paused && d.Speed != "" && d.Speed != idleSpeed {
		speed = "↓ " + d.Speed + "/s"
	}
	left := ""
	if d.Left != "" {
		left = "剩余 " + d.Left
	}
	facts := joinNonEmpty(" · ", progress(d), speed, left)
	return Reply{Text: Lines(Line(Strong(icon+head)), Line(Plain(facts))), Image: d.Image}
}

// transferView is one transfer job with a line per file.
func transferView(j TransferJob) Reply {
	text := Lines(Line(Strong("📦 "+taskName(j.Title, j.Season))), Line(Plain(filesDone(j))))
	for i, f := range j.Files[:min(len(j.Files), maxTransferLines)] {
		name := fmt.Sprintf("文件 %d", i+1)
		if f.Episode > 0 {
			name = fmt.Sprintf("E%02d", f.Episode)
		}
		text = append(text, Line(Plain(fmt.Sprintf(fileStateText[f.State], name))))
	}
	if remaining := len(j.Files) - maxTransferLines; remaining > 0 {
		text = append(text, Line(Emphasis(fmt.Sprintf("另有 %d 个文件，全部状态请在 MoviePilot 中查看。", remaining))))
	}
	return Reply{Text: text, Image: j.Image}
}

func progress(d Download) string {
	if d.Paused {
		return fmt.Sprintf("已暂停 %.0f%%", d.Progress)
	}
	return fmt.Sprintf("进度 %.0f%%", d.Progress)
}

func filesDone(j TransferJob) string {
	done := 0
	for _, f := range j.Files {
		if f.State == FileDone {
			done++
		}
	}
	return fmt.Sprintf("已整理 %d/%d 个文件", done, len(j.Files))
}

// taskName is e.g. 《迷途之子!!!!!》第 1 季.
func taskName(title string, season *int) string {
	name := fmt.Sprintf("《%s》", title)
	if season != nil {
		name += fmt.Sprintf("第 %d 季", *season)
	}
	return name
}
