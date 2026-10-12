package flow

import (
	"cmp"
	"fmt"
)

const (
	msgSearchWarning = "确认后会访问站点，并可能按执行时的订阅条件自动下载资源；正在处理的搜索可能被后台合并。这是一次真实的站点搜索哦～"
	msgSearchReceipt = "这里只留最近一次搜索回执；后台搜索照常进行，离开或关闭这里也一样；已提交的下载会保留，进度请用 /tasks 和订阅详情核对哦～"
	msgStopWarning   = "只请求停止本次搜索里还没提交下载的工作；已提交的下载和订阅都会保留。已发出的站点请求可能还要一会儿才结束哦～"
)

func (e *Engine) subSearchConfirmation(sess session, sub Subscription, problem string) Reply {
	sess.subSearchAsk, sess.subSearchPage = &sub, subSearchPromptPage
	sess.pages, sess.listing = nil, ""
	e.store.put(sess)
	text := Lines(Heading(Plain("🔍 立即搜索订阅")), Line(Strong(taskName(truncate(sub.Title, listTitleRunes), sub.Season))),
		Line(Plain(subScope(sub))), Line(Strong("规格 "), Plain(subSpecifications(sub))), Divider(), Callout(Plain(msgSearchWarning)))
	if sub.State == statePaused {
		text = append(text, Line(Plain("这条订阅已暂停；本次仍会搜索，订阅保持暂停。要停止已受理的搜索，请用「停止本次搜索」。")))
	}
	if problem != "" {
		text = append(text, Line(Plain(problem)))
	}
	return Reply{Text: text, Image: sub.Poster, Buttons: [][]Button{
		{{Label: "确认搜索", Data: data(sess.id, actionSubSearchStart, sub.ID), Style: StylePrimary}},
		{backTo(sess.id, actionRefreshSubDetail, sub.ID)},
	}}
}

func subSearchView(sess session) Reply {
	run := sess.subSearch
	text := Lines(Heading(Plain("🔍 订阅搜索")), Line(Strong(taskName(truncate(run.base.Title, listTitleRunes), run.base.Season))))
	text = append(text, subSearchReceiptText(*run)...)
	if run.last != nil {
		text = append(text, subSearchBatchText(*run)...)
	}
	if run.problem != "" {
		text = append(text, Line(Plain(run.problem)))
	}
	text = append(text, Remark(msgSearchReceipt))
	if run.on {
		text = append(text, Remark(msgFollowing))
	} else if run.last != nil && run.last.active() {
		text = append(text, Remark(fmt.Sprintf("未自动刷新；点「刷新」只读取状态，「继续刷新」最多跟踪 %d 分钟。", int(FollowFor.Minutes()))))
	}
	reply := Reply{Text: text, Image: run.base.Poster, Buttons: subSearchRows(sess)}
	if run.on {
		reply.Follow = data(sess.id, actionSubSearchFollow, run.sequence)
	}
	return reply
}

func subSearchReceiptText(run subscriptionSearch) Text {
	var text Text
	switch run.outcome {
	case searchAccepted:
		text = Lines(Line(Plain("已提交一次单订阅搜索，以下只跟踪本次新建批次哦～")))
	case searchOngoing:
		text = Lines(Group("已有搜索正在处理"), Line(Plain("后台合并到了已有批次，本次没有新建批次。查看和停止它请到 MoviePilot 操作哦～")))
	case searchRefused:
		text = Lines(Group("未能提交搜索"), Line(Plain(run.message)))
	default:
		text = Lines(Group("搜索结果待核对"), Line(Plain("没能确认搜索是否已受理，为免重复只提交了一次。请先用 /subscribe 和 /tasks 核对，或到 MoviePilot 查看哦～")))
	}
	return append(text, subSearchStopText(run)...)
}

func subSearchStopText(run subscriptionSearch) Text {
	switch run.stop {
	case searchAccepted:
		return Lines(Group("停止请求已提交"), Line(Plain("MoviePilot 已受理停止请求，是否已经停下请刷新核对哦～")))
	case searchRefused:
		return Lines(Group("未能提交停止请求"), Line(Plain(run.stopNote)), Line(Plain("这里只提供刷新核对哦～")))
	case searchUnknown:
		return Lines(Group("停止结果待核对"), Line(Plain("没能确认停止请求是否生效，请刷新核对哦～")))
	}
	return nil
}

func subSearchBatchText(run subscriptionSearch) Text {
	batch := run.last
	heading := "本次批次"
	if !run.fresh {
		heading = "上次核实的批次状态（可能已变化）"
	}
	rows := [][]Span{
		{Plain("批次状态"), Plain(cmp.Or(subExecutionText[batch.State], "未知"))},
		{Plain("当前阶段"), Plain(cmp.Or(subExecutionText[batch.Phase], "未知"))},
		{Plain("已处理订阅"), Plain(fmt.Sprintf("%d / %d", batch.Processed, batch.Total))},
		{Plain("搜索完成"), Plain(fmt.Sprint(batch.Finished))}, {Plain("执行失败"), Plain(fmt.Sprint(batch.Failed))},
		{Plain("已停止"), Plain(fmt.Sprint(batch.Cancelled))}, {Plain("已跳过"), Plain(fmt.Sprint(batch.Skipped))},
	}
	text := Lines(Group(heading), Table([]string{"项目", "状态"}, rows...), Small(Plain("更新时间 "+subTime(batch.Updated))))
	if batch.HasError {
		text = append(text, Line(Plain("MoviePilot 返回了执行提示，具体原因请到后台查看；这里不展示错误原文哦～")))
	}
	return text
}

func subSearchRows(sess session) [][]Button {
	run := sess.subSearch
	var rows [][]Button
	if run.batchID != "" {
		row := []Button{{Label: "刷新", Data: data(sess.id, actionSubSearchRead, run.sequence)}}
		if run.on {
			row = append(row, Button{Label: "停止刷新", Data: data(sess.id, actionSubSearchPause, run.sequence)})
		} else if run.last == nil || !run.last.terminal() {
			row = append(row, Button{Label: "继续刷新", Data: data(sess.id, actionSubSearchView, run.sequence)})
		}
		rows = append(rows, row)
	}
	if run.canStop() {
		rows = append(rows, []Button{{Label: "停止本次搜索", Data: data(sess.id, actionSubSearchStop, run.sequence)}})
	}
	return append(rows, append([]Button{backTo(sess.id, actionRefreshSubDetail, run.base.ID)}, browseRow(sess.id)...))
}

func subSearchStopConfirmation(sess session) Reply {
	run := sess.subSearch
	text := Lines(Heading(Plain("🔍 停止本次搜索")), Line(Strong(taskName(truncate(run.base.Title, listTitleRunes), run.base.Season))), Line(Plain(msgStopWarning)))
	return Reply{Text: text, Image: run.base.Poster, Buttons: [][]Button{
		{{Label: "确认停止搜索", Data: data(sess.id, actionSubSearchCancel, run.sequence), Style: StyleDanger}},
		{backTo(sess.id, actionSubSearchRead, run.sequence)},
	}}
}
