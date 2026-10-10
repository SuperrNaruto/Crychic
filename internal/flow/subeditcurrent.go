package flow

import "strings"

// Restoring a field retains backend expressions exactly, without exposing
// their contents or changing any other draft field.
func (e *Engine) restoreSubEditField(sess session) Reply {
	if editFieldName(sess.subEdit.field) == "" {
		return Reply{Notice: msgInvalidChoice}
	}
	sess = copySubEdit(sess)
	edit := sess.subEdit
	edit.draft.copyField(edit.base.conditions(), edit.field)
	edit.forgetWords(edit.field)
	return e.subEditMenu(sess)
}

func (edit *subscriptionEdit) forgetWords(field int) {
	if field == editInclude {
		edit.includeWords = ""
	}
	if field == editExclude {
		edit.excludeWords = ""
	}
}

func subEditRestoreRow(sess session) []Button {
	return []Button{{Label: "恢复原值", Data: data(sess.id, actionSubEditRestore, sess.subEdit.options)}}
}

func (edit subscriptionEdit) fieldStatus() string {
	before := edit.valueText(edit.base.conditions(), edit.field)
	if edit.base.conditions().changedField(edit.draft, edit.field) {
		return "原值 · " + before + "\n草稿 · " + edit.draftText(edit.field) + "（未保存）"
	}
	return "当前 · " + before
}

func (edit subscriptionEdit) optionCurrent(choice subscriptionSpec) bool {
	if edit.field == editUpgrade {
		return upgradeText(edit.draft, edit.base.Kind) == choice.name
	}
	return edit.valueText(edit.draft, edit.field) == choice.name
}

func (edit *subscriptionEdit) chooseSpec(choice subscriptionSpec) {
	if edit.optionCurrent(choice) {
		return
	}
	if edit.valueText(edit.base.conditions(), edit.field) == choice.name {
		edit.draft.copyField(edit.base.conditions(), edit.field)
		return
	}
	edit.setExpression(choice.expression)
}

func knownSpec(expression string, choices []subscriptionSpec) string {
	for _, choice := range choices {
		if expression == choice.expression || expression == choice.name {
			return choice.name
		}
	}
	// Exact legacy strings are safe labels, not rewritten matching rules.
	for _, label := range []string{"WEB-DL", "WEBRip", "1080p", "720p", "2160p", "4K", "HDR", "SDR", "BluRay", "REMUX"} {
		if strings.EqualFold(expression, label) {
			return label
		}
	}
	return "已设置后台表达式（保留原值）"
}
