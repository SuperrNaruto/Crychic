package flow

import (
	"context"
	"slices"
)

const (
	actionSubSearch       = "qs"
	actionSubSearchStart  = "qc"
	actionSubSearchView   = "qv"
	actionSubSearchRead   = "qr"
	actionSubSearchFollow = "qf"
	actionSubSearchPause  = "qu"
	actionSubSearchStop   = "qx"
	actionSubSearchCancel = "qy"

	msgSubSearchChange = "订阅条件或状态已变化，请核对后重新确认搜索哦～"
	msgSubSearchUnread = "暂时无法读取本次搜索状态，点「刷新」只会重新核对，不会重新搜索哦～"
	msgSubSearchScope  = "批次范围暂时无法核实，没有开放停止操作，请到 MoviePilot 核对哦～"
)

func subSearchAction(action string) bool {
	switch action {
	case actionSubSearch, actionSubSearchStart, actionSubSearchView, actionSubSearchRead,
		actionSubSearchFollow, actionSubSearchPause, actionSubSearchStop, actionSubSearchCancel:
		return true
	}
	return false
}

// Search controls, including read-only Follow, must belong to this screen.
// Old subscription controls cannot act behind a search confirmation.
func subSearchChoice(sess session, raw, action string) (bool, bool) {
	if !subSearchAction(action) && sess.subSearchPage == "" {
		return false, false
	}
	if action == actionSubSearchFollow {
		return sess.subSearchPage == subSearchStatusPage && sess.screen.reply.Follow == raw, true
	}
	return hasData(sess.screen.reply.Buttons, raw), true
}

func (e *Engine) chooseSubSearch(ctx context.Context, sess session, p press) (Reply, bool) {
	if !subSearchAction(p.action) {
		return Reply{}, false
	}
	if p.action == actionSubSearch {
		return e.askSubSearch(ctx, sess, p.arg), true
	}
	if p.action == actionSubSearchStart {
		return e.startSubSearch(ctx, sess, p.arg), true
	}
	if sess.subSearch == nil || sess.subSearch.sequence != p.arg {
		return Reply{Notice: msgInvalidChoice}, true
	}
	return e.chooseSubSearchRun(ctx, copySubSearch(sess), p), true
}

func (e *Engine) chooseSubSearchRun(ctx context.Context, sess session, p press) Reply {
	switch p.action {
	case actionSubSearchView:
		sess.subSearch.on, sess.subSearch.until = true, e.now().Add(FollowFor)
		return e.refreshSubSearch(ctx, sess)
	case actionSubSearchRead:
		return e.refreshSubSearch(ctx, sess)
	case actionSubSearchFollow:
		return e.followSubSearch(ctx, sess)
	case actionSubSearchPause:
		sess.subSearch.on = false
		return e.showSubSearch(sess)
	case actionSubSearchStop:
		return e.askStopSubSearch(ctx, sess)
	case actionSubSearchCancel:
		return e.stopSubSearch(ctx, sess)
	}
	return Reply{Notice: msgInvalidChoice}
}

func (e *Engine) askSubSearch(ctx context.Context, sess session, id int) Reply {
	index := sess.subAt(id)
	if index < 0 {
		return Reply{Notice: msgInvalidChoice}
	}
	base := sess.subs[index]
	if run := sess.subSearch; run != nil && run.base.same(base) && !run.canSearchAgain() {
		return e.refreshSubSearch(ctx, copySubSearch(sess))
	}
	sub, notice := e.searchSubscription(ctx, base)
	if notice != "" {
		return Reply{Notice: notice}
	}
	return e.subSearchConfirmation(sess, sub, "")
}

func (e *Engine) searchSubscription(ctx context.Context, base Subscription) (Subscription, string) {
	if base.Created == "" {
		return Subscription{}, msgSubCheck
	}
	subs, err := e.backend.Subscriptions(ctx)
	if err != nil {
		e.log.Warn("subscription search preflight unavailable", "subscription", base.ID)
		return Subscription{}, msgSubCheck
	}
	index := slices.IndexFunc(subs, base.same)
	if index < 0 {
		return Subscription{}, msgSubChanged
	}
	return subs[index], ""
}

func (e *Engine) startSubSearch(ctx context.Context, sess session, id int) Reply {
	if sess.subSearchAsk == nil || sess.subSearchAsk.ID != id {
		return Reply{Notice: msgInvalidChoice}
	}
	base := *sess.subSearchAsk
	sub, notice := e.searchSubscription(ctx, base)
	if notice != "" {
		return Reply{Notice: notice}
	}
	if !base.conditions().same(sub.conditions()) || base.State != sub.State || base.Total != sub.Total {
		return e.subSearchConfirmation(sess, sub, msgSubSearchChange)
	}
	sequence := 1
	if sess.subSearch != nil {
		sequence = sess.subSearch.sequence + 1
	}
	// Consume the confirmation before any side effect. The receipt, not the
	// old prompt, survives back navigation, failed delivery and unknown writes.
	sess.subSearch = &subscriptionSearch{sequence: sequence, base: sub, outcome: searchUnknown}
	sess.subSearchAsk, sess.ticket = nil, 0
	e.store.put(sess)
	result, err := e.backend.SearchSubscription(ctx, sub)
	return e.subSearchSubmitted(ctx, sess, subscriptionSearchResult{receipt: result, err: err})
}

type subscriptionSearchResult struct {
	receipt SubscriptionSearchReceipt
	err     error
}

func (e *Engine) subSearchSubmitted(ctx context.Context, sess session, result subscriptionSearchResult) Reply {
	sess = copySubSearch(sess)
	run := sess.subSearch
	if result.err != nil {
		if message, safe := UserMessage(result.err); safe {
			run.outcome, run.message = searchRefused, message
		}
		return e.showSubSearch(sess)
	}
	if result.receipt.Ongoing {
		run.outcome = searchOngoing
		return e.showSubSearch(sess)
	}
	run.outcome, run.batchID = searchAccepted, result.receipt.BatchID
	run.on, run.until = true, e.now().Add(FollowFor)
	return e.refreshSubSearch(ctx, sess)
}

func subSearchButtons(sess session, sub Subscription) []Button {
	buttons := []Button{{Label: "立即搜索", Data: data(sess.id, actionSubSearch, sub.ID)}}
	run := sess.subSearch
	if run == nil || !run.base.same(sub) {
		return buttons
	}
	if !run.canSearchAgain() {
		buttons = nil
	}
	return append(buttons, Button{Label: "查看本次搜索", Data: data(sess.id, actionSubSearchView, run.sequence)})
}
