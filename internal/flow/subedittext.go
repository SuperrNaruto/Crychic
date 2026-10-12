package flow

import (
	"fmt"
	"slices"
	"strings"
)

const (
	editMenu = iota
	editQuality
	editResolution
	editEffect
	editGroups
	editSites
	editInclude
	editExclude
	editStart
	editUpgrade
	editReview
	editConflicts
)

var subEditFields = []struct {
	id   int
	name string
}{
	{editResolution, "分辨率"}, {editQuality, "资源类型"}, {editEffect, "画面格式"},
	{editGroups, "规则组"}, {editSites, "站点"}, {editInclude, "包含条件"},
	{editExclude, "排除条件"}, {editStart, "起始集"}, {editUpgrade, "洗版模式"},
}

func editFieldName(id int) string {
	for _, field := range subEditFields {
		if field.id == id {
			return field.name
		}
	}
	return ""
}

func (e *Engine) subEditMenu(sess session) Reply {
	sess = copySubEdit(sess)
	sess.subEdit.field = editMenu
	sess.pages, sess.listing = nil, ""
	e.store.put(sess)
	edit := sess.subEdit
	text := edit.heading(msgSubEditTitle)
	status := "没有未保存的修改，选一项开始编辑吧～"
	if count := edit.changeCount(); count > 0 {
		status = fmt.Sprintf("有 %d 项未保存。查看修改并确认后才会生效哦～", count)
	}
	text = append(text, Line(Plain(status)))
	if edit.problem != "" {
		text = append(text, Line(Plain(edit.problem)))
	}
	var buttons []Button
	for _, field := range subEditFields {
		if field.id == editStart && edit.base.Kind != TV {
			continue
		}
		label := field.name
		if edit.base.conditions().changedField(edit.draft, field.id) {
			label += "（已修改）"
		}
		text = append(text, Line(Strong(label+" "), Plain(edit.draftText(field.id))))
		buttons = append(buttons, Button{Label: field.name, Data: data(sess.id, actionSubEditField, field.id)})
	}
	rows := grid(buttons, menuColumns)
	rows = append(rows, []Button{{Label: "查看修改", Data: data(sess.id, actionSubEditReview, 0)}}, []Button{cancelButton(sess.id)})
	text = append(text, Fold(msgFoldSummary, Sentence("只保存后续匹配条件，已有下载保持原样；要补搜请用「立即搜索」。原有后台表达式请到 MoviePilot 查看。")))
	return Reply{Text: text, Image: edit.base.Poster, Buttons: rows}
}

func (edit subscriptionEdit) changeCount() int {
	n := 0
	for _, field := range subEditFields {
		if edit.base.conditions().changedField(edit.draft, field.id) {
			n++
		}
	}
	return n
}

func (edit subscriptionEdit) heading(title string) Text {
	return Lines(Heading(Plain(title)), Line(Strong(taskName(truncate(edit.base.Title, listTitleRunes), edit.base.Season))))
}

func (edit subscriptionEdit) valueText(value SubscriptionConditions, field int) string {
	switch field {
	case editQuality:
		return namedSpec(value.Quality, qualityChoices)
	case editResolution:
		return namedSpec(value.Resolution, resolutionChoices)
	case editEffect:
		return namedSpec(value.Effect, effectChoices)
	case editGroups:
		return edit.groupText(value)
	case editSites:
		return edit.siteText(value.Sites)
	case editInclude:
		return conditionText(value.Include)
	case editExclude:
		return conditionText(value.Exclude)
	case editStart:
		return fmt.Sprintf("从第 %d 集开始", max(value.StartEpisode, 1))
	case editUpgrade:
		return upgradeText(value, edit.base.Kind)
	}
	return "未知"
}

func conditionText(expression string) string {
	if expression == "" {
		return "不限（未设置）"
	}
	return "已设置（内容不在聊天中展开）"
}

func upgradeText(value SubscriptionConditions, kind Kind) string {
	if !value.BestVersion {
		return "普通订阅"
	}
	if kind == TV && value.BestVersionFull {
		return "全集洗版"
	}
	if kind == TV {
		return "分集洗版"
	}
	return "洗版订阅"
}

func (edit subscriptionEdit) groupText(value SubscriptionConditions) string {
	if len(value.FilterGroups) > 0 {
		return fmt.Sprintf("已选 %d 组 · %s", len(value.FilterGroups), subSetting(strings.Join(value.FilterGroups, " · ")))
	}
	if value.BestVersion {
		return "沿用全局洗版规则组"
	}
	return "沿用全局普通订阅规则组"
}

func (edit subscriptionEdit) siteText(ids []int) string {
	if len(ids) == 0 {
		return "沿用 MoviePilot 默认订阅站点"
	}
	var names []string
	for _, id := range ids {
		i := slices.IndexFunc(edit.sites, func(s SearchSite) bool { return s.ID == id })
		if i >= 0 {
			names = append(names, edit.sites[i].Name)
		}
	}
	if len(names) != len(ids) {
		return fmt.Sprintf("已选 %d 个站点（目录尚未核对）", len(ids))
	}
	return fmt.Sprintf("已选 %d 个站点 · %s", len(ids), subSetting(strings.Join(names, " · ")))
}

func (e *Engine) reviewSubEdit(sess session) Reply {
	edit := *sess.subEdit
	if edit.base.Kind == TV && !validEditStart(max(edit.draft.StartEpisode, 1), edit.base.Total) {
		return Reply{Notice: "起始集超出了最新范围，请先修改起始集哦～"}
	}
	before := edit.base.conditions()
	if before.same(edit.draft) {
		return Reply{Notice: "还没有要保存的修改哦～"}
	}
	text := edit.heading("📚 确认修改订阅条件")
	for _, field := range subEditFields {
		old, new := edit.valueText(before, field.id), edit.draftText(field.id)
		if before.changedField(edit.draft, field.id) {
			text = append(text, Group(field.name), Line(Plain("原来 · "+old)), Line(Plain("改为 · "+new)))
		}
	}
	if before.BestVersion != edit.draft.BestVersion && len(edit.draft.FilterGroups) == 0 {
		text = append(text, Line(Plain("生效规则组 · "+edit.groupText(edit.draft))))
	}
	text = append(text, Callout(Plain("后台后续调度会使用新条件，并可能搜索下载。")),
		Remark("只提交上述改动，已取得集数、缺集数和洗版优先级保持原样；到货情况以媒体库为准。"))
	edit.field, sess.subEdit = editReview, &edit
	e.store.put(sess)
	return Reply{Text: text, Image: edit.base.Poster, Buttons: [][]Button{{{Label: "确认保存", Data: data(sess.id, actionSubEditSave, edit.base.ID)}}}}
}

func (edit subscriptionEdit) draftText(field int) string {
	if field == editInclude && edit.includeWords != "" {
		return "字面关键词 · " + edit.includeWords
	}
	if field == editExclude && edit.excludeWords != "" {
		return "字面关键词 · " + edit.excludeWords
	}
	return edit.valueText(edit.draft, field)
}
