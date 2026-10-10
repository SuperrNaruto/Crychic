package flow

import (
	"context"
	"slices"
)

// A write receipt is not an editable draft. It retains only enough memory
// to compare explicit changes; neither navigation nor checking can submit it.
type subscriptionEditReceipt struct {
	base  Subscription
	want  SubscriptionConditions
	sites []SearchSite
	token int
}

func (e *Engine) retainSubEditReceipt(sess session, edit subscriptionEdit) session {
	sess = consumedSubEdit(sess)
	sess.editOptions++
	sess.subReceipt = &subscriptionEditReceipt{
		base: edit.base, want: edit.draft, sites: slices.Clone(edit.sites), token: sess.editOptions,
	}
	e.store.put(sess)
	return sess
}

func (e *Engine) checkSubEditResult(ctx context.Context, sess session, token int) Reply {
	receipt := sess.subReceipt
	if receipt == nil || token != receipt.token {
		return Reply{Notice: msgInvalidChoice}
	}
	current, notice := e.editableSubscription(ctx, sess, receipt.base)
	if notice == "" {
		e.keepSubEditScope(ctx, current)
	}
	text := Lines(Heading(Plain("订阅修改核对结果")), Line(Strong(taskName(truncate(receipt.base.Title, listTitleRunes), receipt.base.Season))))
	if notice != "" {
		text = append(text, Line(Plain(notice)))
	}
	before := receipt.base.conditions()
	view := subscriptionEdit{base: receipt.base, draft: receipt.want, sites: receipt.sites}
	for _, field := range subEditFields {
		if !before.changedField(receipt.want, field.id) {
			continue
		}
		status := "暂时无法确认"
		if notice == "" {
			status = receipt.fieldResult(current.conditions(), field.id)
		}
		text = append(text, Group(field.name), Line(Strong(status)), Line(Plain("期望 · "+view.valueText(receipt.want, field.id))))
	}
	text = append(text, Remark("只核对当前值，没有再次保存。当前值相同不证明上次请求一定成功，其他值也不代表请求从未成功；条件内容不在这里展开。"))
	return Reply{Text: text, Image: receipt.base.Poster, Buttons: [][]Button{subEditCheckRow(sess), browseRow(sess.id)}}
}

func (receipt subscriptionEditReceipt) fieldResult(current SubscriptionConditions, field int) string {
	if !current.changedField(receipt.want, field) {
		return "已生效（当前值与本次修改一致）"
	}
	if !current.changedField(receipt.base.conditions(), field) {
		return "仍为原值（当前未见本次修改）"
	}
	return "已变为其他值（与原值、本次修改均不同）"
}

func subEditCheckRow(sess session) []Button {
	if sess.subReceipt == nil {
		return nil
	}
	return []Button{{Label: "核对结果", Data: data(sess.id, actionSubEditCheck, sess.subReceipt.token)}}
}
