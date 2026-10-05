package flow

import (
	"context"
	"fmt"
	"slices"
)

const (
	actionSubs        = "j" // lists subscriptions
	actionAskCancel   = "v" // arg: index into session subs; asks to confirm
	actionUnsubscribe = "z" // arg: index into session subs; cancels it

	msgNoSubs    = "当前没有订阅。"
	msgSubsTitle = "📚 订阅列表"
)

// stateText words MoviePilot's subscription states.
var stateText = map[string]string{
	"N": "待处理",
	"R": "订阅中",
	"P": "待定",
	"S": "已暂停",
}

// Subscriptions opens the subscription list.
func (e *Engine) Subscriptions(ctx context.Context, actor Actor) Reply {
	return e.listSubs(ctx, e.store.create(actor, nil))
}

// chooseSubs applies the subscription list actions; ok is false for others.
func (e *Engine) chooseSubs(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionSubs:
		return e.listSubs(ctx, sess), true
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

// listSubs shows every subscription; the ones the user asked for get a
// cancel button.
func (e *Engine) listSubs(ctx context.Context, sess session) Reply {
	subs, err := e.backend.Subscriptions(ctx)
	if err != nil {
		e.store.take(sess.id)
		return e.failure("subscriptions", err)
	}
	sess.subs, sess.mine = subs, e.watcher.Requested(sess.owner.UserID)
	e.store.put(sess)
	if len(subs) == 0 {
		return Reply{Text: Sentence(msgNoSubs), Buttons: [][]Button{{homeButton(sess.id)}}}
	}
	view := listView{heading: Line(Strong(fmt.Sprintf("%s（%d）", msgSubsTitle, len(subs)))), footer: []Button{homeButton(sess.id)}}
	for i, s := range subs {
		s.Title = truncate(s.Title, listTitleRunes)
		mine := slices.Contains(sess.mine, s.ID)
		entry := listEntry{text: Lines(subLine(i+1, s, mine))}
		if mine {
			label := fmt.Sprintf("取消 %d", i+1)
			entry.buttons = []Button{{Label: label, Data: data(sess.id, actionAskCancel, i)}}
		}
		view.entries = append(view.entries, entry)
	}
	return e.listPages(sess, view)
}

// subLine is e.g. "1. 《绝命毒师》第 2 季 · 订阅中 · 缺 3/13 集 · 你请求的".
func subLine(n int, s Subscription, mine bool) Block {
	missing := ""
	if s.Kind == TV && s.Lack > 0 {
		missing = fmt.Sprintf("缺 %d 集", s.Lack)
		if s.Total > 0 {
			missing = fmt.Sprintf("缺 %d/%d 集", s.Lack, s.Total)
		}
	}
	who := ""
	if mine {
		who = "你请求的"
	}
	facts := joinNonEmpty(" · ", stateText[s.State], missing, who)
	return Line(Plain(fmt.Sprintf("%d. ", n)), Strong(fmt.Sprintf("《%s》", s.Title)), Plain(seasonSuffix(s)+" · "+facts))
}

// seasonSuffix follows a 《title》, e.g. 第 2 季.
func seasonSuffix(s Subscription) string {
	if s.Season == nil {
		return ""
	}
	return fmt.Sprintf("第 %d 季", *s.Season)
}

// askCancel asks before deleting the subscription at index.
func askCancel(sess session, index int) Reply {
	s := sess.subs[index]
	question := fmt.Sprintf("确认取消订阅《%s》%s？MoviePilot 将不再为它搜索下载。", s.Title, seasonSuffix(s))
	return Reply{
		Text:  Lines(Line(Strong(question))),
		Image: s.Poster,
		Buttons: [][]Button{{
			{Label: "确认取消", Data: data(sess.id, actionUnsubscribe, index)},
			{Label: "返回", Data: data(sess.id, actionSubs, 0)},
		}},
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
	done := fmt.Sprintf("✅ 已取消订阅《%s》%s。", s.Title, seasonSuffix(s))
	return Reply{Text: Sentence(done), Buttons: [][]Button{{
		{Label: "返回订阅列表", Data: data(sess.id, actionSubs, 0)}, homeButton(sess.id),
	}}}
}
