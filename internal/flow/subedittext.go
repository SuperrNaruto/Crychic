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
)

var subEditFields = []struct {
	id   int
	name string
}{
	{editResolution, "分辨率"}, {editQuality, "质量"}, {editEffect, "特效"},
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
	text = append(text, Line(Plain("这是未保存的草稿，选一项调整，最后查看修改并确认哦～")))
	var buttons []Button
	for _, field := range subEditFields {
		if field.id == editStart && edit.base.Kind != TV {
			continue
		}
		text = append(text, Line(Strong(field.name+" "), Plain(edit.draftText(field.id))))
		buttons = append(buttons, Button{Label: field.name, Data: data(sess.id, actionSubEditField, field.id)})
	}
	rows := grid(buttons, menuColumns)
	rows = append(rows, []Button{{Label: "查看修改", Data: data(sess.id, actionSubEditReview, 0)}}, []Button{cancelButton(sess.id)})
	text = append(text, Remark("仅保存后续匹配条件；不会主动补搜，不会暂停或删除已有下载。原有后台表达式不在聊天中展开。"))
	return Reply{Text: text, Image: edit.base.Poster, Buttons: rows}
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
	before := edit.base.conditions()
	if before.same(edit.draft) {
		return Reply{Notice: "还没有要保存的修改哦～"}
	}
	text := edit.heading("确认修改订阅条件")
	for _, field := range subEditFields {
		old, new := edit.valueText(before, field.id), edit.draftText(field.id)
		if before.changedField(edit.draft, field.id) {
			text = append(text, Group(field.name), Line(Plain("原来 · "+old)), Line(Plain("改为 · "+new)))
		}
	}
	if before.BestVersion != edit.draft.BestVersion && len(edit.draft.FilterGroups) == 0 {
		text = append(text, Line(Plain("生效规则组 · "+edit.groupText(edit.draft))))
	}
	text = append(text, Remark("只提交上述改动，不重置已取得集数、缺集数或洗版优先级；调整范围不代表剧集已到货。后台后续调度会使用新条件，并可能搜索下载。"))
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
