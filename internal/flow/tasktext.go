package flow

import "fmt"

const (
	msgNoTasks       = "现在没有在下载或整理的任务，大家都歇着呢～"
	msgUnfollowed    = "好，不自动刷新啦～"
	msgRefreshFailed = "⚠️ 暂时拿不到最新进度，我等下再自动试试～"
	msgFollowing     = "正在帮你盯着进度…"
	msgDownloadGone  = "下载任务结束啦（下完了或者被移除了）。"
	msgTransferGone  = "整理任务结束啦，入库情况看入库通知，或者去 MoviePilot 的整理历史瞧瞧～"
	// idleSpeed is how MoviePilot words a stalled download's speed.
	idleSpeed        = "0.0B"
	maxTransferLines = 100
	bytesPerKiB      = 1024
)

// sizeUnits word sizes the way qBittorrent does.
var sizeUnits = []string{"B", "KiB", "MiB", "GiB", "TiB"}

var msgFollowExpired = fmt.Sprintf("已经帮你盯了 %d 分钟，先歇一下～", int(FollowFor.Minutes()))

// fileStateText words each transfer file state with its marker.
var fileStateText = map[FileState]string{
	FileWaiting: "⏳ 等待中",
	FileRunning: "▶️ 整理中",
	FileDone:    "✅ 已完成",
	FileFailed:  "⚠️ 失败",
}

// downloadHead names the download view's table columns.
var downloadHead = []string{"大小", "进度", "速度", "剩余"}

// fileHead names the transfer view's table columns.
var fileHead = []string{"集", "状态"}

// taskList numbers downloads, then transfer jobs, matching session tasks.
func taskList(id uint64, downloads []Download, jobs []TransferJob) listView {
	view := listView{heading: Heading(Plain("📋 正在忙的任务")), footer: []Button{{Label: "关闭", Data: data(id, actionClose, 0)}}}
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
		add(d.Title, d.Season, joinNonEmpty(" · ", EpisodeRanges(d.Episodes), size(d.Size), progress(d)))
	}
	section = "📦 整理"
	for _, j := range jobs {
		add(j.Title, j.Season, filesDone(j))
	}
	return view
}

// downloadView is one download, its size, progress, speed and time left in
// a table, e.g. 17.5 GiB | 37% | 3.1MB/s | 5分12秒.
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
	row := []Span{Plain(size(d.Size)), Plain(percent), Plain(speed), Plain(d.Left)}
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
		text = append(text, Remark(fmt.Sprintf("还有 %d 个文件没列出来，完整状态去 MoviePilot 看看吧～", remaining)))
	}
	return Reply{Text: text, Image: j.Image}
}

func progress(d Download) string {
	if d.Paused {
		return fmt.Sprintf("已暂停 %.0f%%", d.Progress)
	}
	return fmt.Sprintf("进度 %.0f%%", d.Progress)
}

// size is e.g. "17.5 GiB", "" when unknown.
func size(bytes float64) string {
	if bytes <= 0 {
		return ""
	}
	unit := 0
	for bytes >= bytesPerKiB && unit < len(sizeUnits)-1 {
		bytes /= bytesPerKiB
		unit++
	}
	return fmt.Sprintf("%.1f %s", bytes, sizeUnits[unit])
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
