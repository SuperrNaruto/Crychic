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
	FileWaiting: "⏳ 等待中",
	FileRunning: "▶️ 整理中",
	FileDone:    "✅ 已完成",
	FileFailed:  "⚠️ 失败",
}

// downloadHead names the download view's table columns.
var downloadHead = []string{"进度", "速度", "剩余"}

// fileHead names the transfer view's table columns.
var fileHead = []string{"集", "状态"}

// taskList numbers downloads, then transfer jobs, matching session tasks.
func taskList(id uint64, downloads []Download, jobs []TransferJob) listView {
	view := listView{heading: Heading(Plain("📋 进行中的任务")), footer: []Button{{Label: "关闭", Data: data(id, actionClose, 0)}}}
	section := "⬇️ 下载"
	add := func(title string, season *int, line string) {
		n := len(view.entries) + 1
		title = truncate(title, listTitleRunes)
		text := Lines(entry(n, Strong(taskName(title, season)), truncate(line, listTitleRunes)))
		if section != "" {
			text = append(Lines(Group(section)), text...)
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

// downloadView is one download, its progress, speed and time left in a
// table, e.g. 37% | 3.1MB/s | 5分12秒.
func downloadView(d Download) Reply {
	icon := "⬇️ "
	if d.Paused {
		icon = "⏸️ "
	}
	head := joinNonEmpty(" ", taskName(d.Title, d.Season), EpisodeRanges(d.Episodes))
	percent := fmt.Sprintf("%.0f%%", d.Progress)
	if d.Paused {
		percent = "已暂停 " + percent
	}
	speed := ""
	if !d.Paused && d.Speed != "" && d.Speed != idleSpeed {
		speed = d.Speed + "/s"
	}
	row := []Span{Plain(percent), Plain(speed), Plain(d.Left)}
	return Reply{Text: Lines(Heading(Plain(icon+head)), Table(downloadHead, row)), Image: d.Image}
}

// transferView is one transfer job with a table row per file.
func transferView(j TransferJob) Reply {
	shown := j.Files[:min(len(j.Files), maxTransferLines)]
	rows := make([][]Span, 0, len(shown))
	for i, f := range shown {
		name := fmt.Sprintf("文件 %d", i+1)
		if f.Episode > 0 {
			name = fmt.Sprintf("E%02d", f.Episode)
		}
		rows = append(rows, []Span{Plain(name), Plain(fileStateText[f.State])})
	}
	text := Lines(Heading(Plain("📦 "+taskName(j.Title, j.Season))), Line(Plain(filesDone(j))))
	if len(rows) > 0 {
		text = append(text, Table(fileHead, rows...))
	}
	if remaining := len(j.Files) - maxTransferLines; remaining > 0 {
		text = append(text, Remark(fmt.Sprintf("另有 %d 个文件，全部状态请在 MoviePilot 中查看。", remaining)))
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
