package flow

import (
	"context"
	"fmt"
	"slices"
)

type subscriptionSpec struct{ name, expression string }

// These expressions follow MoviePilot v3.1.1's built-in video rules. The
// backend evaluates them; Crychic never searches a site to validate a choice.
var resolutionChoices = []subscriptionSpec{
	{"2160p / 4K", "4k|2160p|x2160"}, {"1080p", "1080[pi]|x1080"}, {"720p", "720[pi]|x720"},
}

var qualityChoices = []subscriptionSpec{
	{"WEB-DL / WEBRip", "WEB-?DL|WEB-?RIP"}, {"BluRay", "Blu-?Ray"}, {"REMUX", "REMUX"},
}

var effectChoices = []subscriptionSpec{
	{"HDR", `[\s.]+HDR[\s.]+|HDR10|HDR10\+|HDRVivid`},
	{"杜比视界", `Dolby[\s.]+Vision|DOVI|[\s.]+DV[\s.]+|杜比视界`},
	{"SDR", `[\s.]+SDR[\s.]+`},
}

const (
	upgradeOff = iota + 1
	upgradeEpisodes
	upgradeFull
)

func specChoices(field int) []subscriptionSpec {
	switch field {
	case editResolution:
		return resolutionChoices
	case editQuality:
		return qualityChoices
	case editEffect:
		return effectChoices
	}
	return nil
}

func namedSpec(expression string, choices []subscriptionSpec) string {
	if expression == "" {
		return "不限（未设置）"
	}
	return knownSpec(expression, choices)
}

func (e *Engine) openSubEditField(ctx context.Context, sess session, field int) Reply {
	if editFieldName(field) == "" || field == editStart && sess.subEdit.base.Kind != TV {
		return Reply{Notice: msgInvalidChoice}
	}
	sess = copySubEdit(sess)
	sess.subEdit.field = field
	sess.editOptions++
	sess.subEdit.options = sess.editOptions
	sess.pages, sess.listing = nil, ""
	switch field {
	case editGroups, editSites:
		return e.readSubEditChoices(ctx, sess)
	case editCategory:
		return e.readSubEditCategories(ctx, sess)
	case editInclude, editExclude, editStart:
		e.store.put(sess)
		return subEditPrompt(sess, "")
	}
	return e.subEditOptions(sess)
}

func (e *Engine) subEditOptions(sess session) Reply {
	edit := sess.subEdit
	view := listView{
		heading: Heading(Plain(msgSubEditTitle + " · " + editFieldName(edit.field))),
		note:    "这里只修改草稿，选择后返回编辑菜单哦～", footer: []Button{cancelButton(sess.id)},
		entries: []listEntry{{text: Sentence(edit.fieldStatus())}},
	}
	choices := specChoices(edit.field)
	if edit.field == editUpgrade {
		choices = []subscriptionSpec{{"普通订阅", ""}, {"洗版订阅", ""}}
		if edit.base.Kind == TV {
			choices = []subscriptionSpec{{"普通订阅", ""}, {"分集洗版", ""}, {"全集洗版", ""}}
		}
		view.note = "洗版可能再次下载已有内容；这里只改变匹配模式，当前优先级保持原样，洗版进度以 MoviePilot 为准。"
	} else {
		view.menu = [][]Button{{{Label: "不限", Data: data(sess.id, actionSubEditClear, edit.options)}}}
	}
	view.menu = append(view.menu, subEditRestoreRow(sess))
	for i, choice := range choices {
		item := entry(i+1, Strong(choice.name))
		if edit.optionCurrent(choice) {
			item.Tag = "当前"
		}
		view.entries = append(view.entries, listEntry{
			text:    Lines(item),
			buttons: []Button{{Label: fmt.Sprint(i + 1), Data: subEditChoiceData(sess, i+1)}},
		})
	}
	return e.listPages(sess, view)
}

func (e *Engine) pickSubEdit(sess session, index int) Reply {
	sess = copySubEdit(sess)
	edit := sess.subEdit
	if edit.field == editGroups || edit.field == editSites {
		return e.toggleSubEditChoice(sess, index)
	}
	if edit.field == editCategory {
		return e.pickSubEditCategory(sess, index)
	}
	if edit.field == editUpgrade {
		if !edit.setUpgrade(index) {
			return Reply{Notice: msgInvalidChoice}
		}
		return e.subEditMenu(sess)
	}
	choices := specChoices(edit.field)
	if index < 1 || index > len(choices) {
		return Reply{Notice: msgInvalidChoice}
	}
	edit.chooseSpec(choices[index-1])
	return e.subEditMenu(sess)
}

func (edit *subscriptionEdit) setUpgrade(index int) bool {
	maximum := upgradeFull
	if edit.base.Kind != TV {
		maximum = upgradeEpisodes
	}
	if index < upgradeOff || index > maximum {
		return false
	}
	edit.draft.BestVersion = index != upgradeOff
	if edit.base.Kind == TV && index != upgradeOff {
		edit.draft.BestVersionFull = index == upgradeFull
	}
	return true
}

func (edit *subscriptionEdit) setExpression(value string) {
	switch edit.field {
	case editQuality:
		edit.draft.Quality = value
	case editResolution:
		edit.draft.Resolution = value
	case editEffect:
		edit.draft.Effect = value
	case editInclude:
		edit.draft.Include = value
		edit.includeWords = ""
	case editExclude:
		edit.draft.Exclude = value
		edit.excludeWords = ""
	}
}

func (e *Engine) clearSubEdit(sess session) Reply {
	sess = copySubEdit(sess)
	edit := sess.subEdit
	switch edit.field {
	case editGroups:
		edit.draft.FilterGroups = nil
		return e.subEditDirectory(sess, "")
	case editSites:
		edit.draft.Sites = nil
		return e.subEditDirectory(sess, "")
	case editQuality, editResolution, editEffect, editInclude, editExclude:
		edit.setExpression("")
		return e.subEditMenu(sess)
	case editCategory:
		edit.draft.CategoryID, edit.draft.Category = "", ""
		return e.subEditMenu(sess)
	}
	return Reply{Notice: msgInvalidChoice}
}

func (e *Engine) toggleSubEditChoice(sess session, index int) Reply {
	edit := sess.subEdit
	if edit.field == editSites {
		if index < 1 || index > len(edit.sites) {
			return Reply{Notice: msgInvalidChoice}
		}
		edit.draft.Sites = toggled(edit.draft.Sites, edit.sites[index-1].ID)
	} else {
		if index < 1 || index > len(edit.groups) {
			return Reply{Notice: msgInvalidChoice}
		}
		name := edit.groups[index-1].Name
		edit.draft.FilterGroups = toggleGroup(edit.draft.FilterGroups, name)
	}
	return e.subEditDirectory(sess, subEditChoiceData(sess, index))
}

const subEditOptionBase = 10000

func subEditChoiceData(sess session, index int) string {
	return data(sess.id, actionSubEditPick, sess.subEdit.options*subEditOptionBase+index)
}

func subEditChoice(edit *subscriptionEdit, arg int) (int, bool) {
	index := arg % subEditOptionBase
	return index, arg/subEditOptionBase == edit.options && index > 0
}

func toggleGroup(groups []string, name string) []string {
	if i := slices.Index(groups, name); i >= 0 {
		return slices.Delete(slices.Clone(groups), i, i+1)
	}
	return append(slices.Clip(groups), name)
}
