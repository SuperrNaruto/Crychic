package flow

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	subDetailPageRows  = 20
	subDetailPageItems = 1
	subSettingRunes    = 100
	subTimeLayout      = "2006-01-02 15:04"
)

// Execution states are MoviePilot's last-search projection, never a claim
// that files have finished downloading or are playable.
var subExecutionText = map[string]string{
	"queued": "排队中", "running": "执行中", "matching": "匹配中", "searching": "搜索中",
	"scheduled": "已安排", "waiting_subscription": "等待前一次执行", "waiting_site_budget": "等待站点额度",
	"preparing": "准备下载", "submitting": "提交下载中", "cancelling": "正在停止",
	"finished": "搜索结束", "completed": "搜索结束", "failed": "执行失败",
	"cancelled": "已停止", "skipped": "已跳过",
}

var subProgressHead = []string{"集数", "媒体库", "下载与整理"}

func (d subscriptionDetail) view() listView {
	s := d.sub
	rows, omitted, ambiguous := d.progressRows()
	view := listView{
		heading: Heading(Plain(msgSubDetailTitle + " · " + taskName(truncate(s.Title, listTitleRunes), s.Season))),
		image:   s.Poster, pageItems: subDetailPageItems, note: d.progressNote(omitted, ambiguous),
	}
	if len(rows) == 0 {
		text := append(subDetailText(s), Group("入库与任务"), Line(Plain("还没有可展示的单集信息哦～")))
		view.entries = []listEntry{{text: text}}
		return view
	}
	head := subProgressHead
	if s.Kind == Movie {
		head = []string{"内容", "媒体库", "下载与整理"}
	}
	for start := 0; start < len(rows); start += subDetailPageRows {
		end := min(start+subDetailPageRows, len(rows))
		text := append(subDetailText(s), Group("入库与任务"), Table(head, rows[start:end]...))
		view.entries = append(view.entries, listEntry{text: text})
	}
	return view
}

func subDetailText(s Subscription) Text {
	text := Lines(
		Small(Plain(joinNonEmpty(" · ", s.Year, s.Kind.String(), cmp.Or(stateText[s.State], "状态未知")))),
		Group("订阅设置"),
		Line(Plain(subScope(s))),
		Line(Strong("规格 "), Plain(cmp.Or(subSetting(joinNonEmpty(" · ", s.Resolution, s.Quality, s.Effect)), "未单独设置"))),
		Line(Strong("规则组 "), Plain(cmp.Or(subSetting(strings.Join(s.FilterGroups, " · ")), "未单独设置"))),
		Group("最近一次搜索"),
		Line(Strong("最近搜索 "), Plain(subTime(s.LastSearch))),
		Line(Strong("执行状态 "), Plain(subExecutionState(s.Execution))),
	)
	if s.Execution != nil {
		text = append(text, Line(Strong("下次安排 "), Plain(subTime(s.Execution.NextRun))))
		if s.Execution.HasError {
			text = append(text, Line(Plain("MoviePilot 返回了执行提示，具体原因请到后台查看哦～")))
		}
	}
	return text
}

func subScope(s Subscription) string {
	mode := "普通订阅"
	if s.BestVersion {
		mode = "洗版订阅"
	}
	if s.Kind == Movie {
		return mode
	}
	scope := fmt.Sprintf("从第 %d 集开始", max(s.StartEpisode, 1))
	if s.Total <= 0 {
		return joinNonEmpty(" · ", mode, scope, "总集数暂未确定")
	}
	return fmt.Sprintf("%s · %s · 共 %d 集 · MoviePilot 待获取 %d 集", mode, scope, s.Total, max(s.Lack, 0))
}

func subExecutionState(execution *SubscriptionExecution) string {
	if execution == nil {
		return "暂无执行记录"
	}
	return cmp.Or(subExecutionText[execution.State], "未知")
}

func subTime(t time.Time) string {
	if t.IsZero() {
		return "未知"
	}
	return t.In(calendarZone).Format(subTimeLayout) + "（北京时间）"
}

func subSetting(raw string) string {
	return truncate(strings.Join(strings.Fields(raw), " "), subSettingRunes)
}

func (d subscriptionDetail) progressNote(omitted int, ambiguous bool) string {
	notes := []string{"仅查询，不会触发搜索或下载；点击刷新查看最新状态哦～"}
	if slices.ContainsFunc(d.downloads, func(item Download) bool {
		return d.sameMedia(item.Kind, item.Source, item.MediaID) && d.sameSeason(item.Season)
	}) {
		notes = append(notes, "下载百分比为关联任务的整体进度，不是单集文件进度。")
	}
	for _, read := range []struct {
		name string
		err  error
	}{{"媒体库", d.libraryErr}, {"下载进度", d.downloadsErr}, {"整理进度", d.transfersErr}} {
		if read.err != nil {
			notes = append(notes, read.name+"暂时无法确认。")
		}
	}
	if ambiguous {
		notes = append(notes, "另有季或集数不明的关联任务，未归入单集。")
	}
	if omitted > 0 {
		notes = append(notes, fmt.Sprintf("还有 %d 集未列出，完整情况请到 MoviePilot 查看。", omitted))
	}
	return strings.Join(notes, "\n")
}
