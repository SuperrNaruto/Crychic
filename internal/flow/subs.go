package flow

import (
	"context"
	"fmt"
	"slices"
)

const (
	actionSubs        = "j"  // lists the shown kind's subscriptions afresh
	actionSubsKind    = "sf" // arg: Kind; shows that kind's subscriptions
	actionCancelPick  = "sc" // lists the shown subscriptions the owner may cancel
	actionAskCancel   = "v"  // arg: index into session subs; asks to confirm
	actionUnsubscribe = "z"  // arg: index into session subs; cancels it

	msgNoSubs          = "现在还没有订阅哦～"
	msgNoKindSubs      = "这里还没有%s订阅哦～"
	msgSubsTitle       = "📚 订阅清单"
	msgCancelTitle     = "取消订阅"
	msgCancelPick      = "只能取消你请求的订阅哦，点编号选一个吧。"
	msgNothingToCancel = "这里没有你请求的订阅可以取消哦～"
	msgCancelEffect    = "取消后 MoviePilot 就不会再帮它搜索下载了哦。"
	msgRequestedTag    = "你请求的"
)

// subKinds are the subscription filters, in button order.
var subKinds = []Kind{TV, Movie}

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
		if p.arg < 0 || p.arg >= len(sess.subs) || !slices.Contains(sess.mine, sess.subs[p.arg].ID) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		if p.action == actionAskCancel {
			return askCancel(sess, p.arg), true
		}
		return e.unsubscribe(ctx, sess, sess.subs[p.arg]), true
	}
	return Reply{}, false
}

// listSubs reads every subscription and shows the session's kind, at
// first TV when there are any.
func (e *Engine) listSubs(ctx context.Context, sess session) Reply {
	subs, err := e.backend.Subscriptions(ctx)
	if err != nil {
		e.store.take(sess.id)
		return e.failure("subscriptions", err)
	}
	if len(subs) == 0 && sess.menu {
		return Reply{Notice: msgNoSubs}
	}
	sess.subs, sess.mine = subs, e.watcher.Requested(sess.owner.UserID)
	if len(subs) == 0 {
		e.store.put(sess)
		history := Button{Label: "订阅历史", Data: data(sess.id, actionHistory, int(TV))}
		return Reply{Text: Sentence(msgNoSubs), Buttons: [][]Button{{history, homeButton(sess.id)}}}
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

// subsPage lists the session's subscriptions of its kind; one shared
// button cancels.
func (e *Engine) subsPage(sess session) Reply {
	var view listView
	mine := false
	for _, s := range sess.subs {
		if s.Kind != sess.kind {
			continue
		}
		n := len(view.entries) + 1
		requested := slices.Contains(sess.mine, s.ID)
		mine = mine || requested
		s.Title = truncate(s.Title, listTitleRunes)
		view.entries = append(view.entries, listEntry{text: Lines(subEntry(n, s, requested))})
	}
	view.heading = Heading(Plain(fmt.Sprintf("%s · %s（%d）", msgSubsTitle, sess.kind, len(view.entries))))
	if len(view.entries) == 0 {
		view.entries = []listEntry{{text: Sentence(fmt.Sprintf(msgNoKindSubs, sess.kind))}}
	}
	actions := []Button{{Label: "订阅历史", Data: data(sess.id, actionHistory, int(sess.kind))}}
	if mine {
		actions = append([]Button{{Label: "取消订阅", Data: data(sess.id, actionCancelPick, 0)}}, actions...)
	}
	view.menu = [][]Button{kindRow(sess.id, actionSubsKind), actions}
	view.footer = []Button{homeButton(sess.id)}
	return e.listPages(sess, view)
}

// kindRow switches a list between TV and movies.
func kindRow(id uint64, action string) []Button {
	row := make([]Button, 0, len(subKinds))
	for _, k := range subKinds {
		row = append(row, Button{Label: k.String(), Data: data(id, action, int(k))})
	}
	return row
}

// cancelPicker numbers the shown kind's subscriptions the owner asked for.
func (e *Engine) cancelPicker(sess session) Reply {
	view := listView{heading: Heading(Plain(msgCancelTitle)), note: msgCancelPick, footer: []Button{homeButton(sess.id)}}
	for i, s := range sess.subs {
		if s.Kind != sess.kind || !slices.Contains(sess.mine, s.ID) {
			continue
		}
		n := len(view.entries) + 1
		s.Title = truncate(s.Title, listTitleRunes)
		view.entries = append(view.entries, listEntry{
			text:    Lines(subEntry(n, s, false)),
			buttons: []Button{{Label: fmt.Sprint(n), Data: data(sess.id, actionAskCancel, i)}},
		})
	}
	if len(view.entries) == 0 {
		return Reply{Notice: msgNothingToCancel}
	}
	return e.listPages(sess, view)
}

// subEntry is e.g. "1. **《绝命毒师》第 2 季**" over "_订阅中 · 缺 3/13 集 · 你请求的_".
func subEntry(n int, s Subscription, mine bool) Block {
	missing := ""
	if s.Kind == TV && s.Lack > 0 {
		missing = fmt.Sprintf("缺 %d 集", s.Lack)
		if s.Total > 0 {
			missing = fmt.Sprintf("缺 %d/%d 集", s.Lack, s.Total)
		}
	}
	name := fmt.Sprintf("《%s》%s", s.Title, seasonSuffix(s))
	item := entry(n, Strong(name), stateText[s.State], missing)
	if mine {
		item.Tag = msgRequestedTag
	}
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
		Buttons: [][]Button{{{Label: "确认取消", Data: data(sess.id, actionUnsubscribe, index)}}},
	}
}

// unsubscribe deletes s and forgets its requests, so nobody waits for an
// arrival that will not come.
func (e *Engine) unsubscribe(ctx context.Context, sess session, s Subscription) Reply {
	if err := e.backend.Unsubscribe(ctx, s.ID); err != nil {
		return e.failure("unsubscribe", err)
	}
	if err := e.watcher.Forget(ctx, s.ID); err != nil {
		e.log.Error("cannot forget cancelled subscription", "subscription", s.ID, "err", err)
	}
	e.store.put(sess)
	done := fmt.Sprintf("✅ 好哒，《%s》%s的订阅已经取消啦。", s.Title, seasonSuffix(s))
	return Reply{Text: Sentence(done), Buttons: [][]Button{{
		{Label: "返回订阅列表", Data: data(sess.id, actionSubs, 0)}, homeButton(sess.id),
	}}}
}
