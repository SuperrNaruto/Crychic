package flow

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

func (d requestDetail) summary() string {
	if !d.record.CompletedAt.IsZero() {
		return joinNonEmpty(" · ", d.origin(), "所需范围已到货", subTime(d.record.CompletedAt))
	}
	parts := []string{d.origin(), d.acquisition()}
	if len(d.progress.downloads) > 0 {
		parts = append(parts, fmt.Sprintf("关联下载 %d 个", len(d.progress.downloads)))
	}
	if files := d.fileCount(); files > 0 {
		parts = append(parts, fmt.Sprintf("关联整理 %d 个文件", files))
	}
	if len(d.failures) > 0 {
		parts = append(parts, fmt.Sprintf("关联失败记录 %d 条", len(d.failures)))
	}
	if d.unmapped {
		parts = append(parts, "剧集组映射未确认")
	}
	if d.unknownScope {
		parts = append(parts, "存在季或集数不明的任务")
	}
	if d.progress.sub.Execution != nil && subExecutionText[d.progress.sub.Execution.State] == "" {
		parts = append(parts, "执行状态未知")
	}
	return joinNonEmpty(" · ", parts...)
}

func (d requestDetail) origin() string {
	if d.record.IsDownload {
		return "选种下载"
	}
	return "订阅"
}

func (d requestDetail) acquisition() string {
	if d.record.IsDownload {
		return d.downloadState()
	}
	if d.subErr != nil {
		return "订阅状态暂时无法确认"
	}
	if !d.subscriptionFound {
		return "订阅已结束或已变更 · 到货待核对"
	}
	s := d.progress.sub
	state := cmp.Or(stateText[s.State], "订阅状态未知")
	if s.Kind == TV && s.Total > 0 {
		return fmt.Sprintf("%s · MoviePilot 待获取 %d 集", state, max(s.Lack, 0))
	}
	return state
}

func (d requestDetail) downloadState() string {
	switch {
	case d.ambiguousDownload:
		return "下载实例无法确定"
	case d.progress.downloadsErr != nil:
		return "下载状态暂时无法确认"
	case len(d.progress.downloads) > 0:
		return progress(d.progress.downloads[0])
	default:
		return "暂无可确认的下载任务 · 到货待核对"
	}
}

func (d requestDetail) fileCount() int {
	total := 0
	for _, job := range d.progress.transfers {
		total += len(job.Files)
	}
	return total
}

func (d requestDetail) view() listView {
	rows, omitted, ambiguous := d.progress.progressRows()
	heading := "📋 请求详情 · "
	head := subProgressHead
	if d.progress.sub.Kind == Movie {
		head = []string{"内容", "媒体库", "下载与整理"}
	}
	if !d.record.CompletedAt.IsZero() {
		heading, head = "📥 完成记录 · ", head[:len(head)-1]
		for i := range rows {
			rows[i] = rows[i][:len(head)]
		}
	}
	view := listView{
		heading:   Heading(Plain(heading + taskName(truncate(d.record.Target.Media.Title, listTitleRunes), d.record.Target.Season))),
		pageItems: subDetailPageItems, note: d.note(omitted, ambiguous),
	}
	if len(rows) == 0 {
		view.entries = []listEntry{{text: append(d.detailText(), Line(Plain("暂时没有可核对的单集范围。")))}}
		return view
	}
	for start := 0; start < len(rows); start += subDetailPageRows {
		end := min(start+subDetailPageRows, len(rows))
		text := append(d.detailText(), Group("媒体库与任务"), Table(head, rows[start:end]...))
		view.entries = append(view.entries, listEntry{text: text})
	}
	return view
}

func (d requestDetail) detailText() Text {
	text := Lines(Small(Plain(joinNonEmpty(" · ", d.record.Target.Media.Year, d.record.Target.Media.Kind.String()))), Line(Plain(d.summary())))
	if !d.record.CompletedAt.IsZero() {
		return append(text, Line(Plain(d.completedScope())), Line(Plain("已确认所需范围到货；完成记录不代表媒体库当前可见。")))
	}
	if d.subscriptionFound {
		text = append(text, Line(Plain(subScope(d.progress.sub))), Line(Strong("最近搜索 "), Plain(subExecutionState(d.progress.sub.Execution))))
	}
	if d.record.SubscriptionID > 0 && !d.subscriptionFound && d.subErr == nil {
		text = append(text, Line(Plain("关闭订阅不代表到货，下载和整理仍可能继续哦～")))
	}
	return append(text, d.issueText()...)
}

func (read requestSources) note(mode int) string {
	if mode == requestsCompleted {
		return "仅保存启用后的最近 50 条完整范围到货记录；完成记录不代表媒体库当前可见，点开核对哦～"
	}
	parts := []string{"只读核对，不会补搜、下载或重试整理；媒体库可见性请点开查看。"}
	for _, source := range []struct {
		name string
		err  error
	}{{"订阅状态", read.subErr}, {"下载状态", read.tasks.downloadErr}, {"整理状态", read.tasks.jobErr}} {
		if source.err != nil {
			parts = append(parts, source.name+"暂时无法确认。")
		}
	}
	return strings.Join(append(parts, failedHistoryNote(read.failures.Limited, read.failureErr)), "\n")
}

func failedHistoryNote(limited bool, err error) string {
	if err != nil {
		return "失败历史未完整读取；已读到的关联记录仍保留，未知部分不能当作没有异常。"
	}
	if limited {
		return fmt.Sprintf("仅检查最近 %d 条失败整理记录；未匹配到不代表没有更早的异常。", FailedTransferLimit)
	}
	return "失败记录仅按媒体身份和请求范围关联，不代表当前仍然阻塞。"
}

func (d requestDetail) note(omitted int, ambiguous bool) string {
	notes := []string{d.progress.progressNote(omitted, ambiguous || d.unknownScope)}
	if d.record.CompletedAt.IsZero() {
		notes = append(notes, failedHistoryNote(d.limited, d.failureErr))
	}
	if d.subErr != nil {
		notes = append(notes, "订阅状态暂时无法确认。")
	}
	if d.ambiguousDownload {
		notes = append(notes, "下载实例无法确定，未把任一实例的进度归给这个请求。")
	}
	if d.unmapped {
		notes = append(notes, "剧集组映射未确认，未关联默认季集的媒体库、任务或失败记录。")
	}
	if d.unscopedFailures {
		notes = append(notes, "同资源的失败历史缺少下载器实例，未归为本次下载的异常；请到 MoviePilot 后台核对。")
	}
	return strings.Join(notes, "\n")
}

func (d requestDetail) attention() bool {
	if d.uncertain() || len(d.failures) > 0 || d.subscriptionProblem() {
		return true
	}
	if slices.ContainsFunc(d.progress.downloads, func(item Download) bool { return item.Paused }) {
		return true
	}
	return slices.ContainsFunc(d.progress.transfers, func(job TransferJob) bool {
		return slices.ContainsFunc(job.Files, func(file TransferFile) bool { return file.State == FileFailed || file.State == FileUnknown })
	})
}

func (d requestDetail) uncertain() bool {
	return d.subErr != nil || d.progress.downloadsErr != nil || d.progress.transfersErr != nil ||
		d.failureErr != nil || d.limited || d.ambiguousDownload || d.unmapped || d.unknownScope || d.unscopedFailures
}

func (d requestDetail) subscriptionProblem() bool {
	if d.record.IsDownload {
		return false
	}
	if !d.subscriptionFound {
		return !d.arrivedScope()
	}
	s := d.progress.sub
	if s.State == "S" || stateText[s.State] == "" {
		return true
	}
	return s.Execution != nil && (s.Execution.HasError || s.Execution.State == "failed" || subExecutionText[s.Execution.State] == "")
}

func (d requestDetail) completedScope() string {
	if d.record.Target.Media.Kind == Movie {
		return "当时请求范围：电影"
	}
	return fmt.Sprintf("当时请求范围：从第 %d 集开始，至第 %d 集。", max(d.record.Target.StartEpisode, 1), d.record.Total)
}

func (d requestDetail) arrivedScope() bool {
	r := d.record
	if r.Target.Media.Kind == Movie {
		return r.MovieDelivered
	}
	start := max(r.Target.StartEpisode, 1)
	if r.Target.BestVersion || r.Total < start || r.Total-start+1 > len(r.Delivered) {
		return false
	}
	for ep := start; ep <= r.Total; ep++ {
		if !slices.Contains(r.Delivered, ep) {
			return false
		}
	}
	return true
}
