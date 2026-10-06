package flow

import (
	"context"
	"fmt"
)

const actionRetry = "rt"

type readStep string

const (
	readSearch       readStep = "search"
	readSeasons      readStep = "seasons"
	readSubscription readStep = "subscription lookup"
)

// recovery remembers only a failed read. Subscription writes never enter
// this state, and a failed lookup never leaves a confirmable target behind.
type recovery struct {
	step   readStep
	target Target
}

func (e *Engine) readFailure(sess session, retry recovery, err error) Reply {
	sess.retry, sess.target = retry, nil
	e.store.put(sess)
	failure := e.failure(string(retry.step), err)
	rows := [][]Button{{
		{Label: "重试", Data: data(sess.id, actionRetry, 0)},
		researchButton(sess.id), cancelButton(sess.id),
	}}
	if retry.step != readSearch && sess.picked.Media.ID != "" {
		if retry.step == readSubscription {
			context := Line(Plain(fmt.Sprintf("暂时无法确认%s是否已订阅。", targetName(retry.target))))
			failure.Text = append(Lines(context), failure.Text...)
		}
		return sess.picked.replyLines(failure.Text, rows)
	}
	text := Lines(Heading(Plain("🔍 搜索未完成")), Line(Plain(fmt.Sprintf("正在查找「%s」。", sess.query))))
	return Reply{Text: append(text, failure.Text...), Buttons: rows}
}

func (e *Engine) retryRead(ctx context.Context, sess session) Reply {
	var reply Reply
	switch sess.retry.step {
	case readSearch:
		reply = e.search(ctx, sess, sess.query)
	case readSeasons:
		reply = e.offerSeasons(ctx, sess)
	case readSubscription:
		reply = e.retryConfirmation(ctx, sess)
	default:
		return Reply{Notice: msgInvalidChoice}
	}
	return withResearch(reply, sess.id)
}

// Downloads have already been read; only repeat the failed subscription
// lookup, then resume the normal confirmation path.
func (e *Engine) retryConfirmation(ctx context.Context, sess session) Reply {
	target := sess.retry.target
	id, err := e.backend.FindSubscription(ctx, target)
	if err != nil {
		return e.readFailure(sess, sess.retry, err)
	}
	return e.confirmation(ctx, sess, subscription{id: id, target: target})
}

const msgSubmitUnknown = "未能确认订阅结果。请先用 /subscribe 核对，确认没有订阅后再发起请求。"

// submit attempts the write once. A transport/response failure, including
// a missing receipt, is not proof that MoviePilot rejected the request.
func (e *Engine) submit(ctx context.Context, target Target) (int, error) {
	id, err := e.backend.Subscribe(ctx, target)
	if err == nil && id > 0 {
		return id, nil
	}
	if _, safe := UserMessage(err); safe {
		return 0, err
	}
	e.log.Error("subscription result unknown", "media", target.Media.ID, "id", id, "err", err)
	return 0, &UserError{Message: msgSubmitUnknown}
}
