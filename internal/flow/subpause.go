package flow

import (
	"context"
	"slices"
)

const (
	actionPauseSub  = "sp" // arg: subscription id; pauses it
	actionResumeSub = "sr" // arg: subscription id; resumes it

	msgPauseSub   = "暂停订阅"
	msgResumeSub  = "恢复订阅"
	msgPauseOwn   = "只能暂停或恢复你请求的订阅哦～"
	msgPauseCheck = "没能确认改好了没有，点「刷新」看看现在的状态吧～"

	statePaused = "S"
)

// chooseSubPause pauses or resumes a subscription the owner asked for, then
// shows its details read afresh. The button names the subscription by id,
// not by its place in a list a later read may have reordered.
func (e *Engine) chooseSubPause(ctx context.Context, sess session, p press) (Reply, bool) {
	if p.action != actionPauseSub && p.action != actionResumeSub {
		return Reply{}, false
	}
	index := slices.IndexFunc(sess.subs, func(s Subscription) bool { return s.ID == p.arg })
	if index < 0 {
		return Reply{Notice: msgInvalidChoice}, true
	}
	if !slices.Contains(sess.mine, p.arg) {
		return Reply{Notice: msgPauseOwn}, true
	}
	paused := p.action == actionPauseSub
	if err := e.backend.SetSubscriptionPaused(ctx, p.arg, paused); err != nil {
		if message, safe := UserMessage(err); safe {
			return Reply{Notice: message}, true
		}
		e.log.Error("backend call failed", "step", "pause subscription", "err", err)
		return Reply{Notice: msgPauseCheck}, true
	}
	return e.subDetail(ctx, sess, index), true
}

// pauseButton pauses or resumes sub when the owner asked for it.
func pauseButton(sess session, sub Subscription) []Button {
	if !slices.Contains(sess.mine, sub.ID) {
		return nil
	}
	if sub.State == statePaused {
		return []Button{{Label: msgResumeSub, Data: data(sess.id, actionResumeSub, sub.ID)}}
	}
	return []Button{{Label: msgPauseSub, Data: data(sess.id, actionPauseSub, sub.ID)}}
}
