package flow

import (
	"context"
	"fmt"
)

const (
	msgLatestTitle = "🆕 新鲜入库"
	msgNoLatest    = "媒体库最近还没有新东西哦～"
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
		return titled(msgLatestTitle, e.failure("latest", err))
	}
	if len(items) == 0 && sess.menu {
		return Reply{Notice: msgNoLatest}
	}
	e.store.put(sess)
	if len(items) == 0 {
		text := Lines(Heading(Plain(msgLatestTitle)), Line(Plain(msgNoLatest)))
		return Reply{Text: text, Buttons: [][]Button{browseRow(sess.id)}}
	}
	view := listView{heading: Heading(Plain(msgLatestTitle)), footer: browseRow(sess.id)}
	for i, it := range items {
		title := Linked(Strong(fmt.Sprintf("《%s》", truncate(it.Title, listTitleRunes))), it.Link)
		view.entries = append(view.entries, listEntry{text: Lines(entry(i+1, title, it.Year, it.Kind))})
	}
	return e.listPages(sess, view)
}
