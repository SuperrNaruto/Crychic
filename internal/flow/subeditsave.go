package flow

import (
	"context"
	"slices"
)

const (
	msgSubEditDirectoryChanged = "规则组或站点已变化，尚未保存；先刷新订阅，再重新编辑确认哦～"
	msgSubEditDirectoryRead    = "暂时无法核对规则组或站点，草稿还在，没有保存修改哦～"
	msgSubEditConflict         = "后台条件已变化，尚未保存；先刷新订阅，再按最新设置重新编辑哦～"
	msgSubEditUnknown          = "保存结果待核对。请先点「刷新」或用 /subscribe 查看当前条件，不要重复提交哦～"
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
		return e.endSubEdit(sess, "尚未保存", notice)
	}
	current, notice := e.editableSubscription(ctx, sess, edit.base)
	if notice != "" {
		return Reply{Notice: notice}
	}
	if !edit.base.conditions().same(current.conditions()) || edit.base.Total != current.Total {
		return e.endSubEdit(sess, "尚未保存", msgSubEditConflict)
	}
	// Consume before I/O. Even an unknown outcome has no confirmable draft
	// or navigation snapshot from which the same write could be restored.
	sess = consumedSubEdit(sess)
	e.store.put(sess)
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
	persisted := e.keepSubEditScope(ctx, edit.applied())
	current, notice := e.editableSubscription(ctx, sess, edit.base)
	if notice != "" {
		return subEditReceipt(sess, edit.base, subEditOutcome{"修改已受理，等待读取", "MoviePilot 已确认保存，但暂时读不到当前条件；请先刷新核对，不要重复提交哦～"})
	}
	suffix := "后续会按当前条件匹配；这里没有主动补搜，也没有重置下载事实哦～"
	if !e.keepSubEditScope(ctx, current) || !persisted {
		suffix = "条件已保存，但通知范围暂时没能保存；请刷新核对哦～"
	}
	if !current.conditions().same(edit.draft) {
		return subEditReceipt(sess, current, subEditOutcome{"修改已受理，条件又有变化", "MoviePilot 已确认保存，但当前条件与刚提交的草稿不同，请刷新核对哦～"})
	}
	return subEditReceipt(sess, current, subEditOutcome{"✅ 订阅条件已保存", suffix})
}

func (edit subscriptionEdit) applied() Subscription {
	sub := edit.base
	sub.StartEpisode, sub.BestVersion, sub.BestVersionFull = edit.draft.StartEpisode, edit.draft.BestVersion, edit.draft.BestVersionFull
	return sub
}

func (e *Engine) keepSubEditScope(ctx context.Context, sub Subscription) bool {
	if err := e.watcher.RefreshSubscription(ctx, sub); err != nil {
		e.log.Error("cannot persist edited subscription scope", "subscription", sub.ID)
		return false
	}
	return true
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

func (e *Engine) endSubEdit(sess session, heading, message string) Reply {
	sub := sess.subEdit.base
	sess = consumedSubEdit(sess)
	e.store.put(sess)
	return subEditReceipt(sess, sub, subEditOutcome{heading, message})
}

type subEditOutcome struct{ heading, message string }

func subEditReceipt(sess session, sub Subscription, outcome subEditOutcome) Reply {
	text := Lines(Heading(Plain(outcome.heading)), Line(Strong(taskName(truncate(sub.Title, listTitleRunes), sub.Season))), Line(Plain(outcome.message)))
	rows := [][]Button{{{Label: "刷新", Data: data(sess.id, actionRefreshSubDetail, sub.ID)}}, browseRow(sess.id)}
	return Reply{Text: text, Image: sub.Poster, Buttons: rows}
}
