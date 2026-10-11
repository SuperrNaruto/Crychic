package flow

import "fmt"

func (e *Engine) subEditConflictOverview(sess session) Reply {
	sess = copySubEditConflict(sess)
	edit := sess.subEdit
	edit.field, edit.conflict.field = editConflicts, editMenu
	sess.editOptions++
	edit.options, sess.pages = sess.editOptions, nil
	e.store.put(sess)
	text := edit.heading("后台条件已变化 · 草稿已保留")
	text = append(text, Line(Plain("尚未保存。需要选择的冲突按编号处理；未修改的字段将沿用后台新值，其他草稿不变。点「完成处理」接受这些差异后再确认保存哦～")))
	buttons := []Button{}
	for _, field := range subEditFields {
		if !edit.base.conditions().changedField(edit.draft, field.id) && !edit.base.conditions().changedField(edit.conflict.current.conditions(), field.id) {
			continue
		}
		number := ""
		if edit.fieldConflicts(field.id) {
			number = fmt.Sprintf("%d. ", len(buttons)+1)
			buttons = append(buttons, Button{Label: fmt.Sprint(len(buttons) + 1), Data: data(sess.id, actionSubConflictPick, edit.options*subEditOptionBase+field.id)})
		}
		text = append(text, edit.conflictFieldText(number, field.id)...)
	}
	if edit.base.Total != edit.conflict.current.Total {
		text = append(text, Group("总集数已变化"), Line(Plain(fmt.Sprintf("原值 · %d；后台 · %d。起始集将按最新范围校验。", edit.base.Total, edit.conflict.current.Total))))
	}
	rows := grid(buttons, menuColumns)
	rows = append(rows, []Button{{Label: "重新读取", Data: data(sess.id, actionSubConflictReload, edit.options)}},
		[]Button{{Label: "完成处理", Data: data(sess.id, actionSubConflictDone, edit.options)}}, []Button{cancelButton(sess.id)})
	return Reply{Text: text, Image: edit.base.Poster, Buttons: rows}
}

func (edit subscriptionEdit) conflictFieldText(prefix string, field int) Text {
	status := "未修改此项，沿用后台新值"
	if edit.base.conditions().changedField(edit.draft, field) {
		status = "草稿保留"
	}
	if edit.fieldConflicts(field) {
		status = "待选择"
		if keep, chosen := edit.conflict.choices[field]; chosen {
			status = "采用后台"
			if keep {
				status = "保留草稿"
			}
		}
	}
	return Lines(Group(prefix+editFieldName(field)+" · "+status),
		Line(Plain("后台 · "+edit.valueText(edit.conflict.current.conditions(), field))),
		Line(Plain("草稿 · "+edit.draftText(field))))
}

func (e *Engine) subEditConflictField(sess session, field int) Reply {
	sess = copySubEditConflict(sess)
	sess.editOptions++
	edit := sess.subEdit
	edit.options, edit.conflict.field = sess.editOptions, field
	e.store.put(sess)
	text := edit.heading("选择冲突处理方式")
	text = append(text, edit.conflictFieldText("", field)...)
	text = append(text, Remark("这里只决定草稿如何继续，保存仍要最后确认；未知后台表达式请到 MoviePilot 查看。"))
	return Reply{Text: text, Buttons: [][]Button{
		{{Label: "保留草稿", Data: data(sess.id, actionSubConflictKeep, edit.options)},
			{Label: "采用后台", Data: data(sess.id, actionSubConflictAdopt, edit.options)}},
		{cancelButton(sess.id)},
	}}
}
