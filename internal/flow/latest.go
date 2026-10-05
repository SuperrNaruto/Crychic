package flow

import (
	"context"
	"fmt"
)

const (
	msgLatestTitle = "🆕 最新入库"
	msgNoLatest    = "媒体库最近没有新内容。"
)

// Latest lists what reached the media servers most recently.
func (e *Engine) Latest(ctx context.Context, actor Actor) Reply {
	return e.latest(ctx, e.store.create(actor, nil))
}

// latest links each item to where it can be watched.
func (e *Engine) latest(ctx context.Context, sess session) Reply {
	items, err := e.backend.Latest(ctx)
	if err != nil {
		e.store.take(sess.id)
		return e.failure("latest", err)
	}
	e.store.put(sess)
	buttons := [][]Button{{homeButton(sess.id)}}
	if len(items) == 0 {
		return Reply{Text: Sentence(msgNoLatest), Buttons: buttons}
	}
	view := listView{heading: Line(Strong(msgLatestTitle)), footer: buttons[0]}
	for i, it := range items {
		title := Linked(Strong(fmt.Sprintf("《%s》", truncate(it.Title, listTitleRunes))), it.Link)
		text := Lines(Line(Plain(fmt.Sprintf("%d. ", i+1)), title, Plain(" "+joinNonEmpty(" · ", yearOf(it.Year), it.Kind))))
		view.entries = append(view.entries, listEntry{text: text})
	}
	return e.listPages(sess, view)
}

func yearOf(y string) string {
	if y == "" {
		return ""
	}
	return "(" + y + ")"
}
