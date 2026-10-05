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
	idleSpeed = "0.0B"
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
func taskList(id uint64, downloads []Download, jobs []TransferJob) Reply {
	text := Lines(Line(Strong("📋 进行中的任务")))
	var rows [][]Button
	add := func(title string, season *int, line string) {
		n := len(rows) + 1
		text = append(text, Line(Plain(fmt.Sprintf("%d. %s · %s", n, taskName(title, season), line))))
		label := fmt.Sprintf("%d. %s", n, title)
		if season != nil {
			label += fmt.Sprintf(" 第 %d 季", *season)
		}
		rows = append(rows, []Button{{Label: label, Data: data(id, actionTask, n-1)}})
	}
	if len(downloads) > 0 {
		text = append(text, Line(Strong("⬇️ 下载")))
	}
	for _, d := range downloads {
		add(d.Title, d.Season, joinNonEmpty(" · ", EpisodeRanges(d.Episodes), progress(d)))
	}
	if len(jobs) > 0 {
		text = append(text, Line(Strong("📦 整理")))
	}
	for _, j := range jobs {
		add(j.Title, j.Season, filesDone(j))
	}
	rows = append(rows, []Button{cancelButton(id)})
	return Reply{Text: text, Buttons: rows}
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
	for i, f := range j.Files {
		name := fmt.Sprintf("文件 %d", i+1)
		if f.Episode > 0 {
			name = fmt.Sprintf("E%02d", f.Episode)
		}
		text = append(text, Line(Plain(fmt.Sprintf(fileStateText[f.State], name))))
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
