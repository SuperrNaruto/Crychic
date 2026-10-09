package flow

import (
	"context"
	"slices"
)

const (
	actionSubEdit       = "ue"
	actionSubEditField  = "uf"
	actionSubEditPick   = "uv"
	actionSubEditClear  = "uc"
	actionSubEditApply  = "ua"
	actionSubEditReview = "uq"
	actionSubEditSave   = "uw"
	actionSubEditInput  = "ui"

	msgSubEditTitle = "编辑订阅条件"
	msgSubEditOwn   = "只能编辑你请求的订阅哦～"
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
}

func subEditAction(action string) bool {
	switch action {
	case actionSubEdit, actionSubEditField, actionSubEditPick, actionSubEditClear,
		actionSubEditApply, actionSubEditReview, actionSubEditSave, actionSubEditInput:
		return true
	}
	return false
}

func (e *Engine) chooseSubEdit(ctx context.Context, sess session, p press) (Reply, bool) {
	if !subEditAction(p.action) {
		return Reply{}, false
	}
	if p.action == actionSubEdit {
		return e.openSubEdit(ctx, sess, p.arg), true
	}
	if sess.subEdit == nil {
		return Reply{Notice: msgInvalidChoice}, true
	}
	switch p.action {
	case actionSubEditField:
		return e.openSubEditField(ctx, sess, p.arg), true
	case actionSubEditPick:
		return e.chooseSubEditOption(sess, p.arg), true
	case actionSubEditClear:
		return e.clearSubEdit(sess), true
	case actionSubEditApply:
		return e.subEditMenu(sess), true
	case actionSubEditReview:
		return e.reviewSubEdit(sess), true
	case actionSubEditSave:
		return e.saveSubEdit(ctx, sess, p.arg), true
	}
	return Reply{Notice: msgInvalidChoice}, true
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
	if index < 0 || !slices.Contains(sess.mine, id) {
		return Reply{Notice: msgSubEditOwn}
	}
	sub, notice := e.editableSubscription(ctx, sess, sess.subs[index])
	if notice != "" {
		return Reply{Notice: notice}
	}
	sess.subEdit = &subscriptionEdit{base: sub, draft: sub.conditions()}
	return e.subEditMenu(sess)
}

func (e *Engine) editableSubscription(ctx context.Context, sess session, base Subscription) (Subscription, string) {
	subs, err := e.backend.Subscriptions(ctx)
	if err != nil {
		e.log.Warn("subscription edit preflight unavailable", "subscription", base.ID)
		return Subscription{}, msgSubCheck
	}
	i := slices.IndexFunc(subs, base.same)
	if i < 0 {
		return Subscription{}, msgSubChanged
	}
	sub := subs[i]
	if !slices.Contains(e.watcher.Requested(sess.owner.UserID, []Subscription{sub}), sub.ID) {
		return Subscription{}, msgSubEditOwn
	}
	return sub, ""
}

func copySubEdit(sess session) session {
	edit := *sess.subEdit
	edit.draft.FilterGroups = slices.Clone(edit.draft.FilterGroups)
	edit.draft.Sites = slices.Clone(edit.draft.Sites)
	sess.subEdit = &edit
	return sess
}

// Editing is a local form: field screens do not accumulate navigation
// snapshots containing older drafts. 返回 from the menu discards the form.
func (e *Engine) backFromSubEdit(sess session) (Reply, bool) {
	if sess.subEdit == nil || sess.subEdit.field == editMenu {
		return Reply{}, false
	}
	return e.navigate(sess, actionSubEditApply, e.subEditMenu(sess)), true
}
