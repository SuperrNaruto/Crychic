package flow

import (
	"context"
	"slices"
)

const (
	actionPauseSub  = "sp" // arg: index into session subs; pauses it
	actionResumeSub = "sr" // arg: index into session subs; resumes it

	msgPauseSub   = "暂停订阅"
	msgResumeSub  = "恢复订阅"
	msgPauseOwn   = "只能暂停或恢复你请求的订阅哦～"
	msgPauseCheck = "没能确认改好了没有，点「刷新」看看现在的状态吧～"

	statePaused = "S"
)

// chooseSubPause pauses or resumes a subscription the owner asked for, then
// shows its details read afresh.
func (e *Engine) chooseSubPause(ctx context.Context, sess session, p press) (Reply, bool) {
	if p.action != actionPauseSub && p.action != actionResumeSub {
		return Reply{}, false
	}
	if p.arg < 0 || p.arg >= len(sess.subs) {
		return Reply{Notice: msgInvalidChoice}, true
	}
	if !slices.Contains(sess.mine, sess.subs[p.arg].ID) {
		return Reply{Notice: msgPauseOwn}, true
	}
	paused := p.action == actionPauseSub
	if err := e.backend.SetSubscriptionPaused(ctx, sess.subs[p.arg].ID, paused); err != nil {
		if message, safe := UserMessage(err); safe {
			return Reply{Notice: message}, true
		}
		e.log.Error("backend call failed", "step", "pause subscription", "err", err)
		return Reply{Notice: msgPauseCheck}, true
	}
	return e.subDetail(ctx, sess, p.arg), true
}

// pauseButton pauses or resumes sub, shown at index, when the owner asked
// for it.
func pauseButton(sess session, sub Subscription, index int) []Button {
	if !slices.Contains(sess.mine, sub.ID) {
		return nil
	}
	if sub.State == statePaused {
		return []Button{{Label: msgResumeSub, Data: data(sess.id, actionResumeSub, index)}}
	}
	return []Button{{Label: msgPauseSub, Data: data(sess.id, actionPauseSub, index)}}
}
