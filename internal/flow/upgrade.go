package flow

import (
	"context"
	"fmt"
)

const (
	actionUpgrade        = "bv" // asks to subscribe the card's target as an upgrade (洗版)
	actionUpgradeConfirm = "bo" // subscribes it as an upgrade

	msgUpgradeExplain    = "洗版会按 MoviePilot 订阅规则的优先级一直找更好的版本，下到最高优先级才结束；媒体库里已有的也会再下一遍哦～"
	msgUpgradeSubscribed = "ℹ️ 这个已经订阅上啦，要洗版得在 MoviePilot 里改这个订阅哦～"
	msgUpgradeUnchecked  = "⚠️ MoviePilot 暂时没告诉我订阅状态，稍后再点一次吧～"
)

// chooseUpgrade applies the steps of an upgrade subscription.
func (e *Engine) chooseUpgrade(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionUpgrade:
		return e.askUpgrade(ctx, sess), true
	case actionUpgradeConfirm:
		return e.confirmUpgrade(ctx, sess), true
	}
	return Reply{}, false
}

// upgradeRow offers 洗版订阅 on a card whose target is not subscribed yet,
// including one the library already holds.
func upgradeRow(sess session) []Button {
	return []Button{{Label: "洗版订阅", Data: data(sess.id, actionUpgrade, 0)}}
}

// askUpgrade asks to subscribe the card's whole target as an upgrade. The
// target's subscription is read again first: MoviePilot answers a new
// subscription of a subscribed target with the existing row, unchanged, so
// an upgrade could not start that way. A failed read only flashes a notice.
func (e *Engine) askUpgrade(ctx context.Context, sess session) Reply {
	if sess.focus == nil {
		return Reply{Notice: msgInvalidChoice}
	}
	target := Target{Media: sess.focus.Media, Season: sess.focus.Season, BestVersion: true}
	existing, err := e.backend.FindSubscription(ctx, target)
	if err != nil {
		e.log.Warn("subscription check for upgrade failed", "media", target.Media.ID, "err", err)
		return Reply{Notice: msgUpgradeUnchecked}
	}
	if existing != 0 {
		return Reply{Notice: msgUpgradeSubscribed}
	}
	sess.target = &target
	e.store.put(sess)
	question := Line(Strong(fmt.Sprintf("要洗版订阅%s吗？", targetName(target))))
	confirm := Button{Label: "确认洗版", Data: data(sess.id, actionUpgradeConfirm, 0)}
	return sess.picked.replyLines(Lines(question, Line(Plain(msgUpgradeExplain))), [][]Button{{confirm}, {cancelButton(sess.id)}})
}

// confirmUpgrade writes the upgrade subscription once, like confirm.
func (e *Engine) confirmUpgrade(ctx context.Context, sess session) Reply {
	if sess.target == nil || !sess.target.BestVersion {
		return Reply{Notice: msgInvalidChoice}
	}
	if !e.settle(sess) {
		return expiredText(msgExpired)
	}
	target := *sess.target
	id, err := e.submit(ctx, target)
	if err != nil {
		return sess.picked.replyLines(e.failure("subscribe", err).Text, nil)
	}
	ending := e.watch(ctx, sess, subscription{id: id, target: target})
	done := fmt.Sprintf("✅ 帮你订好%s的洗版啦%s ヾ(≧▽≦*)o", targetName(target), ending)
	return sess.picked.reply(Line(Plain(done)), nil)
}
