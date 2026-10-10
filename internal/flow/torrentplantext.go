package flow

import "fmt"

func torrentPlanList(sess session, notice string) listView {
	plan := sess.plan
	target := *sess.focus
	target.Media.Title = truncate(target.Media.Title, listTitleRunes)
	view := listView{
		heading: Heading(Plain(msgTorrentPlan + " · " + targetName(target))),
		source:  torrentPlanView, footer: []Button{cancelButton(sess.id)},
		menu: [][]Button{{{Label: msgPlanRecalculate, Data: data(sess.id, actionTorrentPlanRefresh, 0)}}},
	}
	if notice != "" {
		view.entries = append(view.entries, listEntry{text: Lines(Line(Strong(notice)))})
	}
	if plan.problem != "" {
		view.entries = append(view.entries, listEntry{text: Lines(Line(Plain(plan.problem)))})
		return view
	}
	view.entries = append(view.entries, listEntry{text: planSummary(*plan)}, listEntry{text: planRisks(*plan)})
	for n, index := range plan.batch {
		t := sess.torrents[index]
		item := torrentEntry(n+1, t, "")
		view.entries = append(view.entries, listEntry{text: Lines(item)})
	}
	if len(plan.batch) > 0 {
		view.menu = append([][]Button{{{Label: msgDownloadLabel, Data: data(sess.id, actionTorrentBatchGet, 0)}}}, view.menu...)
	}
	view.note = fmt.Sprintf("建议 %d 个资源；确认下载的是完整资源，不会裁剪已有或重叠文件。尚未入库的下载未计入，请先用 /tasks 核对。", len(plan.batch))
	return view
}

func planSummary(plan torrentPlan) Text {
	required := fmt.Sprintf("E%02d–E%02d", plan.scope.from, plan.scope.count)
	rows := [][]Span{
		{Plain("所需范围"), Plain(required)},
		{Plain("本次库存"), Plain(planRange(plan.scope.held))},
		{Plain("缺集"), Plain(planRange(plan.missing))},
		{Plain("预计可补"), Plain(planRange(plan.covered))},
		{Plain("仍缺"), Plain(planRange(plan.remaining))},
		{Plain("重复已有"), Plain(planRange(plan.repeated))},
		{Plain("方案内重叠"), Plain(planRange(plan.overlap))},
		{Plain("起始集以前"), Plain(planRange(plan.extra))},
	}
	text := Lines(Line(Plain("按 MoviePilot 本次返回库存与资源识别范围计算，不代表种子文件已逐个核实。")), Table([]string{"项目", "集数"}, rows...))
	if len(plan.missing) == 0 {
		text = append(text, Line(Plain("本次返回库存已覆盖所需范围，不需要生成下载方案啦。")))
	} else if len(plan.batch) == 0 {
		text = append(text, Line(Plain("当前结果没有可信资源能补这些缺集，请返回列表人工核对哦。")))
	}
	return text
}

func planRange(episodes []int) string {
	if len(episodes) == 0 {
		return "无"
	}
	return fmt.Sprintf("%s（%d 集）", EpisodeRanges(episodes), len(episodes))
}

func planRisks(plan torrentPlan) Text {
	text := Lines(Line(Plain(planSize(plan))))
	if len(plan.scope.held) == 0 {
		text = append(text, Line(Strong("⚠️ 本次未返回本季库存，请先核对媒体库连接与扫描状态；空结果不保证媒体库正常。")))
	}
	if plan.hitAndRun > 0 {
		text = append(text, Line(Plain(fmt.Sprintf("⚠️ H&R 资源 %d 个；下完需要满足站点保种要求，免费也不代表免保种哦。", plan.hitAndRun))))
	}
	if plan.unseeded > 0 {
		text = append(text, Line(Plain(fmt.Sprintf("⚠️ %d 个资源未报告做种，不能保证可以下载。", plan.unseeded))))
	}
	if plan.excluded > 0 {
		text = append(text, Line(Plain(fmt.Sprintf("已排除 %d 个身份或集数范围不确定的资源；不推断未知整包、多季或特殊剧集组的覆盖。", plan.excluded))))
	}
	if plan.limited {
		text = append(text, Line(Plain(fmt.Sprintf("一次最多建议 %d 个资源，达到上限后仍缺的集数已列出。", maxTickedReleases))))
	}
	return append(text, Remark("优先多补缺集，同覆盖时优先无 H&R、少重复、已知且较小体积，最后按 MoviePilot 优先级；这是建议，不保证最优。确认前会再核对库存，但不能防止核对之后发生变化。"))
}

func planSize(plan torrentPlan) string {
	if plan.unknownSizes > 0 {
		if plan.totalSize > 0 {
			return fmt.Sprintf("已知大小 %s；另有 %d 个资源大小未知，总大小未知。", size(plan.totalSize), plan.unknownSizes)
		}
		return fmt.Sprintf("%d 个资源大小未知，总大小未知。", plan.unknownSizes)
	}
	if len(plan.batch) == 0 {
		return "尚未选择可下载资源。"
	}
	return "总大小 " + size(plan.totalSize)
}
