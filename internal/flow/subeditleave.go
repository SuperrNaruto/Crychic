package flow

import (
	"context"
	"fmt"
)

type subEditDeparture struct {
	action    string
	edit      subscriptionEdit
	reply     Reply
	pages     []Reply
	pageIndex int
}

func (edit subscriptionEdit) dirty() bool {
	return !edit.base.conditions().same(edit.draft)
}

func (e *Engine) askSubEditLeave(sess session, action string) Reply {
	sess = copySubEdit(sess)
	departure := &subEditDeparture{
		action: action, edit: *sess.subEdit, reply: sess.screen.reply,
		pages: sess.pages, pageIndex: sess.pageIndex,
	}
	sess.editOptions++
	sess.subEdit.leaving, sess.subEdit.options = departure, sess.editOptions
	sess.pages = nil
	e.store.put(sess)
	text := sess.subEdit.heading("📚 放弃未保存修改？")
	text = append(text, Line(Plain(fmt.Sprintf("有 %d 项未保存。点「返回」继续编辑；放弃只丢掉草稿，订阅保持原样哦～", sess.subEdit.changeCount()))))
	return Reply{Text: text, Buttons: [][]Button{
		{{Label: "放弃修改", Data: data(sess.id, actionSubEditDiscard, sess.subEdit.options)}},
		{backTo(sess.id, actionBack, 0)},
	}}
}

func (e *Engine) resumeSubEdit(sess session) Reply {
	departure := sess.subEdit.leaving
	edit := departure.edit
	sess.subEdit, sess.pages, sess.pageIndex = &edit, departure.pages, departure.pageIndex
	e.store.put(sess)
	// Reissue confirmation tickets, rather than restoring a pre-exit write.
	return e.navigate(sess, actionSubEditApply, withoutTickets(departure.reply))
}

func (e *Engine) discardSubEdit(ctx context.Context, sess session) Reply {
	if sess.subEdit.leaving == nil {
		return Reply{Notice: msgInvalidChoice}
	}
	action := sess.subEdit.leaving.action
	sess.subEdit = nil
	e.store.put(sess)
	if action == actionCancel {
		return e.cancel(sess)
	}
	return withoutTickets(e.back(ctx, sess))
}

func withoutTickets(reply Reply) Reply {
	rows := make([][]Button, 0, len(reply.Buttons))
	for _, row := range reply.Buttons {
		next := append([]Button{}, row...)
		for i, button := range next {
			id, press, ok := parseData(button.Data)
			if ok && needsTicket(press.action) {
				next[i].Data = data(id, press.action, press.arg)
			}
		}
		rows = append(rows, next)
	}
	reply.Buttons = rows
	return reply
}
