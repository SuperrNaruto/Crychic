package flow

import (
	"context"
	"slices"
)

const (
	msgSubEditDirectoryChanged = "规则组或站点已变化，尚未保存。"
	msgSubEditDirectoryRead    = "暂时无法核对规则组或站点，草稿还在，没有保存修改哦～"
	msgSubEditUnknown          = "保存结果待核对。请点「核对结果」逐项只读检查当前条件，不要重复提交哦～"
)

func (e *Engine) saveSubEdit(ctx context.Context, sess session, id int) Reply {
	edit := *sess.subEdit
	if !edit.confirmable(id) {
		return Reply{Notice: msgInvalidChoice}
	}
	if notice := e.checkSubEditDirectories(ctx, edit); notice != "" {
		if notice == msgSubEditDirectoryRead {
			return Reply{Notice: notice}
		}
		return e.retainSubEditProblem(sess, notice)
	}
	current, notice := e.editableSubscription(ctx, edit.base)
	if notice != "" {
		return Reply{Notice: notice}
	}
	if !edit.base.conditions().same(current.conditions()) || edit.base.Total != current.Total {
		return e.startSubEditConflict(sess, current)
	}
	// Consume before I/O. Even an unknown outcome has no confirmable draft
	// or navigation snapshot from which the same write could be restored.
	sess = e.retainSubEditReceipt(sess, edit)
	err := e.backend.UpdateSubscription(ctx, SubscriptionUpdate{ID: id, Before: edit.base.conditions(), After: edit.draft})
	if err != nil {
		if message, safe := UserMessage(err); safe {
			return subEditReceipt(sess, edit.base, subEditOutcome{"未保存订阅条件", message})
		}
		e.log.Warn("subscription edit outcome unknown", "subscription", id)
		return subEditReceipt(sess, edit.base, subEditOutcome{"保存结果待核对", msgSubEditUnknown})
	}
	return e.readSavedSubEdit(ctx, sess, edit)
}

func (edit subscriptionEdit) confirmable(id int) bool {
	return edit.field == editReview && id == edit.base.ID && !edit.base.conditions().same(edit.draft)
}

func (e *Engine) readSavedSubEdit(ctx context.Context, sess session, edit subscriptionEdit) Reply {
	current, notice := e.editableSubscription(ctx, edit.base)
	if notice != "" {
		return subEditReceipt(sess, edit.base, subEditOutcome{"修改已受理，等待读取", "MoviePilot 已确认保存，但暂时读不到当前条件；请点「核对结果」，不要重复提交哦～"})
	}
	if !current.conditions().same(edit.draft) {
		return subEditReceipt(sess, current, subEditOutcome{"修改已受理，条件又有变化", "MoviePilot 已确认保存，但当前条件与刚提交的草稿不同，请刷新核对哦～"})
	}
	return subEditReceipt(sess, current, subEditOutcome{"✅ 订阅条件已保存", "后续会按当前条件匹配；这里没有主动补搜，也没有重置下载事实哦～"})
}

func consumedSubEdit(sess session) session {
	sess.subEdit, sess.pages, sess.listing = nil, nil, ""
	sess.target, sess.chosen, sess.ticket = nil, nil, 0
	sess.lastChoice, sess.lastReply = "", Reply{}
	sess.history = slices.Clone(sess.history)
	for i, old := range sess.history {
		if old.state.listing == subscriptionList {
			sess.history = sess.history[:i+1]
			sess.history[i].reload = true
			return sess
		}
	}
	sess.history = nil
	return sess
}

func (e *Engine) retainSubEditProblem(sess session, message string) Reply {
	sess = copySubEdit(sess)
	sess.subEdit.problem = message + " 草稿已保留，重新选择变化的目录项后再查看修改哦～"
	return e.subEditMenu(sess)
}

type subEditOutcome struct{ heading, message string }

func subEditReceipt(sess session, sub Subscription, outcome subEditOutcome) Reply {
	text := Lines(Heading(Plain(outcome.heading)), Line(Strong(taskName(truncate(sub.Title, listTitleRunes), sub.Season))), Line(Plain(outcome.message)))
	rows := [][]Button{{{Label: "刷新", Data: data(sess.id, actionRefreshSubDetail, sub.ID)}}, browseRow(sess.id)}
	rows = append([][]Button{subEditCheckRow(sess)}, rows...)
	return Reply{Text: text, Image: sub.Poster, Buttons: rows}
}
