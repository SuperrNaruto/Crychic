package flow

import (
	"context"
	"fmt"
)

const (
	msgLatestTitle = "🆕 新鲜入库"
	msgNoLatest    = "媒体库最近还没有新东西哦～"
	msgOtherKind   = "其他"
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
	view := listView{
		heading:   Heading(Plain(msgLatestTitle)),
		entries:   latestEntries(items),
		footer:    browseRow(sess.id),
		pageItems: compactPageItems,
	}
	return e.listPages(sess, view)
}

// latestEntries groups items by kind in the order the kinds first appear,
// one line each, e.g. "1. 《沙丘》 · 2021" under 电影（3）.
func latestEntries(items []LibraryItem) []listEntry {
	var kinds []string
	byKind := map[string][]LibraryItem{}
	for _, it := range items {
		kind := it.Kind
		if kind == "" {
			kind = msgOtherKind
		}
		if _, seen := byKind[kind]; !seen {
			kinds = append(kinds, kind)
		}
		byKind[kind] = append(byKind[kind], it)
	}
	var entries []listEntry
	for _, kind := range kinds {
		section := fmt.Sprintf("%s（%d）", kind, len(byKind[kind]))
		for _, it := range byKind[kind] {
			title := Linked(Strong(fmt.Sprintf("《%s》", truncate(it.Title, listTitleRunes))), it.Link)
			item := entry(len(entries)+1, title, it.Year)
			item.Brief = true
			entries = append(entries, listEntry{text: Lines(item), section: section})
		}
	}
	return entries
}
