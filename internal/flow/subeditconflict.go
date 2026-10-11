package flow

import (
	"context"
	"maps"
)

const (
	actionSubConflictPick   = "up"
	actionSubConflictKeep   = "uk"
	actionSubConflictAdopt  = "ub"
	actionSubConflictDone   = "ug"
	actionSubConflictReload = "ur"
)

type subEditConflict struct {
	current Subscription
	choices map[int]bool // explicit per-conflict decision: true keeps the draft
	field   int
}

func subEditConflictAction(action string) bool {
	switch action {
	case actionSubConflictPick, actionSubConflictKeep, actionSubConflictAdopt,
		actionSubConflictDone, actionSubConflictReload:
		return true
	}
	return false
}

func (e *Engine) startSubEditConflict(sess session, current Subscription) Reply {
	sess = copySubEdit(sess)
	sess.subEdit.conflict = &subEditConflict{current: current, choices: map[int]bool{}}
	sess.subEdit.problem = "后台条件已变化，草稿已保留；处理差异后仍需要重新确认保存哦～"
	return e.subEditConflictOverview(sess)
}

func copySubEditConflict(sess session) session {
	sess = copySubEdit(sess)
	conflict := *sess.subEdit.conflict
	conflict.choices = maps.Clone(conflict.choices)
	sess.subEdit.conflict = &conflict
	return sess
}

func (e *Engine) chooseSubEditConflict(ctx context.Context, sess session, p press) Reply {
	if sess.subEdit.conflict == nil {
		return Reply{Notice: msgInvalidChoice}
	}
	switch p.action {
	case actionSubConflictPick:
		field, ok := subEditChoice(sess.subEdit, p.arg)
		if !ok || !sess.subEdit.fieldConflicts(field) {
			return Reply{Notice: msgInvalidChoice}
		}
		return e.subEditConflictField(sess, field)
	case actionSubConflictKeep, actionSubConflictAdopt:
		return e.decideSubEditConflict(sess, p.action == actionSubConflictKeep)
	case actionSubConflictDone:
		return e.finishSubEditConflict(ctx, sess)
	case actionSubConflictReload:
		current, notice := e.editableSubscription(ctx, sess.subEdit.base)
		if notice != "" {
			return Reply{Notice: notice}
		}
		return e.startSubEditConflict(sess, current)
	}
	return Reply{Notice: msgInvalidChoice}
}

func (edit subscriptionEdit) fieldConflicts(field int) bool {
	if editFieldName(field) == "" {
		return false
	}
	base, current := edit.base.conditions(), edit.conflict.current.conditions()
	return base.changedField(edit.draft, field) && base.changedField(current, field) && current.changedField(edit.draft, field)
}

func (e *Engine) decideSubEditConflict(sess session, keep bool) Reply {
	sess = copySubEditConflict(sess)
	conflict := sess.subEdit.conflict
	if !sess.subEdit.fieldConflicts(conflict.field) {
		return Reply{Notice: msgInvalidChoice}
	}
	conflict.choices[conflict.field] = keep
	return e.subEditConflictOverview(sess)
}

func (e *Engine) finishSubEditConflict(ctx context.Context, sess session) Reply {
	edit := sess.subEdit
	for _, field := range subEditFields {
		if _, chosen := edit.conflict.choices[field.id]; edit.fieldConflicts(field.id) && !chosen {
			return Reply{Notice: "还有冲突项没选好，请先逐项处理哦～"}
		}
	}
	current, notice := e.editableSubscription(ctx, edit.base)
	if notice != "" {
		return Reply{Notice: notice}
	}
	if !current.conditions().same(edit.conflict.current.conditions()) || current.Total != edit.conflict.current.Total {
		return e.startSubEditConflict(sess, current)
	}
	sess = copySubEditConflict(sess)
	sess.subEdit.applyConflict()
	return e.subEditMenu(sess)
}

func (edit *subscriptionEdit) applyConflict() {
	base, current := edit.base.conditions(), edit.conflict.current.conditions()
	for _, field := range subEditFields {
		changed := base.changedField(edit.draft, field.id)
		adopt := !changed || edit.fieldConflicts(field.id) && !edit.conflict.choices[field.id]
		if adopt {
			edit.draft.copyField(current, field.id)
			edit.forgetWords(field.id)
		}
	}
	edit.base = edit.conflict.current
	edit.conflict, edit.problem = nil, "差异已处理，草稿尚未保存。请重新查看修改并确认哦～"
}
