package flow

import (
	"context"
	"slices"
)

const (
	actionSubEdit        = "ue"
	actionSubEditField   = "uf"
	actionSubEditPick    = "uv"
	actionSubEditClear   = "uc"
	actionSubEditApply   = "ua"
	actionSubEditReview  = "uq"
	actionSubEditSave    = "uw"
	actionSubEditInput   = "ui"
	actionSubEditRestore = "uo"
	actionSubEditDiscard = "ud"
	actionSubEditCheck   = "uy"

	msgSubEditTitle = "编辑订阅条件"
)

type subscriptionEdit struct {
	base         Subscription
	draft        SubscriptionConditions
	field        int
	groups       []SubscriptionRuleGroup
	sites        []SearchSite
	includeWords string
	excludeWords string
	options      int // current option-screen generation
	leaving      *subEditDeparture
	conflict     *subEditConflict
	problem      string
}

func subEditAction(action string) bool {
	switch action {
	case actionSubEdit, actionSubEditField, actionSubEditPick, actionSubEditClear,
		actionSubEditApply, actionSubEditReview, actionSubEditSave, actionSubEditInput,
		actionSubEditRestore, actionSubEditDiscard, actionSubEditCheck:
		return true
	}
	return subEditConflictAction(action)
}

func (e *Engine) chooseSubEdit(ctx context.Context, sess session, p press) (Reply, bool) {
	if !subEditAction(p.action) {
		return Reply{}, false
	}
	if p.action == actionSubEdit {
		return e.openSubEdit(ctx, sess, p.arg), true
	}
	if p.action == actionSubEditCheck {
		return e.checkSubEditResult(ctx, sess, p.arg), true
	}
	if sess.subEdit == nil {
		return Reply{Notice: msgInvalidChoice}, true
	}
	return e.editSubCondition(ctx, sess, p), true
}

func (e *Engine) editSubCondition(ctx context.Context, sess session, p press) Reply {
	switch p.action {
	case actionSubEditField:
		return e.openSubEditField(ctx, sess, p.arg)
	case actionSubEditPick:
		return e.chooseSubEditOption(sess, p.arg)
	case actionSubEditClear:
		return e.clearSubEdit(sess)
	case actionSubEditApply:
		return e.subEditMenu(sess)
	case actionSubEditReview:
		return e.reviewSubEdit(sess)
	case actionSubEditSave:
		return e.saveSubEdit(ctx, sess, p.arg)
	case actionSubEditRestore:
		return e.restoreSubEditField(sess)
	case actionSubEditDiscard:
		return e.discardSubEdit(ctx, sess)
	}
	return e.chooseSubEditConflict(ctx, sess, p)
}

func (e *Engine) chooseSubEditOption(sess session, arg int) Reply {
	index, ok := subEditChoice(sess.subEdit, arg)
	if !ok {
		return Reply{Notice: msgInvalidChoice}
	}
	return e.pickSubEdit(sess, index)
}

func (e *Engine) openSubEdit(ctx context.Context, sess session, id int) Reply {
	index := sess.subAt(id)
	if index < 0 {
		return Reply{Notice: msgInvalidChoice}
	}
	sub, notice := e.editableSubscription(ctx, sess.subs[index])
	if notice != "" {
		return Reply{Notice: notice}
	}
	sess.subEdit = &subscriptionEdit{base: sub, draft: sub.conditions()}
	return e.subEditMenu(sess)
}

func (e *Engine) editableSubscription(ctx context.Context, base Subscription) (Subscription, string) {
	subs, err := e.backend.Subscriptions(ctx)
	if err != nil {
		e.log.Warn("subscription edit preflight unavailable", "subscription", base.ID)
		return Subscription{}, msgSubCheck
	}
	i := slices.IndexFunc(subs, base.same)
	if i < 0 {
		return Subscription{}, msgSubChanged
	}
	return subs[i], ""
}

func copySubEdit(sess session) session {
	edit := *sess.subEdit
	edit.draft.FilterGroups = slices.Clone(edit.draft.FilterGroups)
	edit.draft.Sites = slices.Clone(edit.draft.Sites)
	sess.subEdit = &edit
	return sess
}

// Field navigation keeps the draft. Leaving a changed form needs an
// explicit discard, without putting a writable draft in navigation history.
func (e *Engine) backFromSubEdit(sess session) (Reply, bool) {
	if sess.subEdit == nil {
		return Reply{}, false
	}
	if sess.subEdit.leaving != nil {
		return e.resumeSubEdit(sess), true
	}
	if sess.subEdit.conflict != nil && sess.subEdit.conflict.field != editMenu {
		return e.navigate(sess, actionSubEditApply, e.subEditConflictOverview(sess)), true
	}
	if sess.subEdit.field == editMenu {
		if !sess.subEdit.dirty() {
			return Reply{}, false
		}
		return e.navigate(sess, actionSubEditApply, e.askSubEditLeave(sess, actionBack)), true
	}
	sess = copySubEdit(sess)
	sess.subEdit.conflict = nil
	return e.navigate(sess, actionSubEditApply, e.subEditMenu(sess)), true
}
