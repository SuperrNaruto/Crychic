package flow

import (
	"context"
	"fmt"
	"slices"
)

const (
	actionSubs        = "j"  // lists the shown kind's subscriptions afresh
	actionSubsKind    = "sf" // arg: Kind; shows that kind's subscriptions
	actionCancelPick  = "sc" // lists the shown subscriptions to cancel
	actionAskCancel   = "v"  // arg: subscription id; asks to confirm
	actionUnsubscribe = "z"  // arg: subscription id; cancels it

	msgNoSubs          = "现在还没有订阅哦～"
	msgNoKindSubs      = "这里还没有%s订阅哦～"
	msgSubsTitle       = "📚 订阅清单"
	msgCancelTitle     = "📚 取消订阅"
	msgCancelPick      = "点编号选一个要取消的订阅吧。"
	msgNothingToCancel = "这里没有可以取消的订阅哦～"
	msgCancelEffect    = "取消后 MoviePilot 会停止为它搜索下载哦。"
)

// subKinds are the subscription filters, in button order.
var subKinds = []Kind{TV, Movie}

// stateSubscribed is the usual state, left unsaid in subscription lists.
const stateSubscribed = "R"

// stateText words MoviePilot's subscription states.
var stateText = map[string]string{
	"N": "待处理",
	"R": "订阅中",
	"P": "待定",
	"S": "已暂停",
}

// Subscriptions opens the subscription list.
func (e *Engine) Subscriptions(ctx context.Context, actor Actor) Reply {
	sess := e.store.create(actor, nil)
	return e.shown(sess.id, e.listSubs(ctx, sess))
}

// chooseSubs applies the subscription list actions; ok is false for others.
func (e *Engine) chooseSubs(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionSubs:
		return e.listSubs(ctx, sess), true
	case actionSubsKind:
		if !slices.Contains(subKinds, Kind(p.arg)) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		sess.kind = Kind(p.arg)
		return e.subsPage(sess), true
	case actionCancelPick:
		return e.cancelPicker(sess), true
	case actionAskCancel, actionUnsubscribe:
		i := sess.subAt(p.arg)
		if i < 0 {
			return Reply{Notice: msgInvalidChoice}, true
		}
		if p.action == actionAskCancel {
			return askCancel(sess, i), true
		}
		return e.unsubscribe(ctx, sess, sess.subs[i]), true
	}
	return Reply{}, false
}

// listSubs reads every subscription and shows the session's kind, at
// first TV when there are any.
func (e *Engine) listSubs(ctx context.Context, sess session) Reply {
	subs, err := e.backend.Subscriptions(ctx)
	if err != nil {
		e.store.take(sess.id)
		return titled(msgSubsTitle, e.failure("subscriptions", err))
	}
	if len(subs) == 0 && sess.menu {
		return Reply{Notice: msgNoSubs}
	}
	sess.subs = subs
	if len(subs) == 0 {
		sess.pages, sess.listing, sess.pageIndex = nil, subscriptionList, 0
		e.store.put(sess)
		kind := sess.kind
		if kind == 0 {
			kind = TV
		}
		history := Button{Label: "订阅历史", Data: data(sess.id, actionHistory, int(kind))}
		text := Lines(Heading(Plain(msgSubsTitle)), Line(Plain(msgNoSubs)))
		return Reply{Text: text, Buttons: [][]Button{{history}, browseRow(sess.id)}}
	}
	if sess.kind == 0 {
		sess.kind = firstKind(subs)
	}
	return e.subsPage(sess)
}

func firstKind(subs []Subscription) Kind {
	for _, s := range subs {
		if s.Kind == TV {
			return TV
		}
	}
	return Movie
}

// subsPage lists the session's subscriptions of its kind; number buttons
// open read-only details, and one shared button cancels.
func (e *Engine) subsPage(sess session) Reply {
	view := listView{source: subscriptionList, pageItems: compactPageItems}
	for _, s := range sess.subs {
		if s.Kind != sess.kind {
			continue
		}
		n := len(view.entries) + 1
		s.Title = truncate(s.Title, listTitleRunes)
		view.entries = append(view.entries, listEntry{
			text:    Lines(subEntry(n, s)),
			buttons: []Button{{Label: fmt.Sprint(n), Data: data(sess.id, actionSubDetail, s.ID)}},
		})
	}
	view.heading = Heading(Plain(fmt.Sprintf("%s · %s（%d）", msgSubsTitle, sess.kind, len(view.entries))))
	actions := []Button{{Label: "订阅历史", Data: data(sess.id, actionHistory, int(sess.kind))}}
	if len(view.entries) == 0 {
		view.entries = []listEntry{{text: Sentence(fmt.Sprintf(msgNoKindSubs, sess.kind))}}
	} else {
		actions = append([]Button{{Label: "取消订阅", Data: data(sess.id, actionCancelPick, 0)}}, actions...)
	}
	view.menu = [][]Button{kindRow(sess.id, actionSubsKind, sess.kind), actions}
	view.footer = browseRow(sess.id)
	return e.listPages(sess, view)
}

// kindRow switches a list between TV and movies, the shown kind marked
// like the shown weekday, e.g. ·电视剧·.
func kindRow(id uint64, action string, shown Kind) []Button {
	row := make([]Button, 0, len(subKinds))
	for _, k := range subKinds {
		row = append(row, Button{Label: markShown(k.String(), k == shown), Data: data(id, action, int(k))})
	}
	return row
}

// markShown marks the label of the view shown now among its siblings.
func markShown(label string, shown bool) string {
	if shown {
		return "·" + label + "·"
	}
	return label
}

// cancelPicker numbers the shown kind's subscriptions.
func (e *Engine) cancelPicker(sess session) Reply {
	view := listView{heading: Heading(Plain(msgCancelTitle)), note: msgCancelPick, footer: browseRow(sess.id), pageItems: compactPageItems}
	for _, s := range sess.subs {
		if s.Kind != sess.kind {
			continue
		}
		n := len(view.entries) + 1
		s.Title = truncate(s.Title, listTitleRunes)
		view.entries = append(view.entries, listEntry{
			text:    Lines(subEntry(n, s)),
			buttons: []Button{{Label: fmt.Sprint(n), Data: data(sess.id, actionAskCancel, s.ID)}},
		})
	}
	if len(view.entries) == 0 {
		return Reply{Notice: msgNothingToCancel}
	}
	return e.listPages(sess, view)
}

// subEntry is one line, e.g. "1. **《绝命毒师》第 2 季** · _缺 3/13 集_",
// naming the state when it is other than 订阅中.
func subEntry(n int, s Subscription) Block {
	missing := ""
	if s.Kind == TV && s.Lack > 0 {
		missing = fmt.Sprintf("缺 %d 集", s.Lack)
		if s.Total > 0 {
			missing = fmt.Sprintf("缺 %d/%d 集", s.Lack, s.Total)
		}
	}
	state := stateText[s.State]
	if s.State == stateSubscribed {
		state = ""
	}
	name := fmt.Sprintf("《%s》%s", s.Title, seasonSuffix(s))
	item := entry(n, Strong(name), state, missing)
	item.Brief = true
	return item
}

// seasonSuffix follows a 《title》, e.g. 第 2 季.
func seasonSuffix(s Subscription) string {
	if s.Season == nil {
		return ""
	}
	return fmt.Sprintf("第 %d 季", *s.Season)
}

// askCancel asks before deleting the subscription at index; 返回 leads
// back to the picker.
func askCancel(sess session, index int) Reply {
	s := sess.subs[index]
	question := fmt.Sprintf("真的要取消订阅《%s》%s吗？", s.Title, seasonSuffix(s))
	return Reply{
		Text:    Lines(Heading(Plain(msgCancelTitle)), Line(Strong(question)), Line(Plain(msgCancelEffect))),
		Image:   s.Poster,
		Buttons: [][]Button{{{Label: "确认取消", Data: data(sess.id, actionUnsubscribe, s.ID)}}},
	}
}

// unsubscribe deletes s after rechecking its identity.
func (e *Engine) unsubscribe(ctx context.Context, sess session, s Subscription) Reply {
	if notice := e.checkSubscription(ctx, s); notice != "" {
		return Reply{Notice: notice}
	}
	nav := append([]Button{backTo(sess.id, actionSubs, 0)}, browseRow(sess.id)...)
	if err := e.backend.Unsubscribe(ctx, s.ID); err != nil {
		failed := titled(msgCancelTitle, e.failure("unsubscribe", err))
		failed.Buttons = [][]Button{nav}
		return failed
	}
	e.store.put(sess)
	done := fmt.Sprintf("好哒，《%s》%s的订阅已经取消啦。", s.Title, seasonSuffix(s))
	return Reply{Text: Lines(Heading(Plain("✅ 已取消订阅")), Line(Plain(done))), Buttons: [][]Button{nav}}
}

// subAt is the index in the session's list of the subscription with id, -1
// when it is not there. Buttons name a subscription by its id, never by its
// place in a list a later read may have reordered: a stale 确认取消 or 暂停订阅
// must not reach another subscription.
func (sess session) subAt(id int) int {
	return slices.IndexFunc(sess.subs, func(s Subscription) bool { return s.ID == id })
}
