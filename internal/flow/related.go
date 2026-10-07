package flow

import (
	"context"
	"fmt"
)

const (
	actionRelated = "r" // lists media recommended alongside the picked one
	actionSeries  = "n" // lists the picked movie's series

	// maxSeriesParts bounds a series list; long ones still fit a page.
	maxSeriesParts = 20

	msgNoRelated = "没找到相似的作品呢～"
	msgNoSeries  = "没找到这部电影的系列呢～"
)

// chooseRelated applies the browse-onward actions; ok is false for others.
func (e *Engine) chooseRelated(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionRelated:
		return e.browse(ctx, sess, false), true
	case actionSeries:
		return e.browse(ctx, sess, true), true
	}
	return Reply{}, false
}

// relatedRow offers to browse on from the picked media: its releases once a
// target is chosen, similar media, and for a movie its series.
func relatedRow(sess session) []Button {
	var row []Button
	if sess.focus != nil {
		row = append(row, torrentButton(sess.id))
	}
	row = append(row, Button{Label: "相似推荐", Data: data(sess.id, actionRelated, 0)})
	if sess.picked.Media.Kind == Movie {
		row = append(row, Button{Label: "同系列", Data: data(sess.id, actionSeries, 0)})
	}
	return row
}

// browse lists media related to the picked one, or its series, to pick from
// like a chart. Browsing is optional, so nothing found or a failed
// lookup only flashes a notice and the card stays.
func (e *Engine) browse(ctx context.Context, sess session, series bool) Reply {
	media := sess.picked.Media
	if media.ID == "" {
		return Reply{Notice: msgInvalidChoice}
	}
	list, limit, empty := e.backend.Related, MaxResults, msgNoRelated
	heading := fmt.Sprintf("🔍 喜欢「%s」的话，也许会喜欢这些～", media.Title)
	if series {
		list, limit, empty = e.backend.Series, maxSeriesParts, msgNoSeries
		heading = fmt.Sprintf("🎬「%s」的同系列作品", media.Title)
	}
	found, err := list(ctx, media)
	if err != nil {
		return e.notice("related", err)
	}
	if len(found) == 0 {
		return Reply{Notice: empty}
	}
	sess.results = found[:min(len(found), limit)]
	sess.seasons, sess.library, sess.target, sess.chosen, sess.focus = nil, Library{}, nil, nil, nil
	return e.listPages(sess, relatedView(sess, heading))
}

// relatedView lists the related media like a chart: posters in a gallery,
// each entry opens to its synopsis, a number button picks it.
func relatedView(sess session, heading string) listView {
	view := listView{heading: Heading(Plain(heading)), footer: []Button{cancelButton(sess.id)}}
	look := pickLook{year: true, kind: mixedKinds(sess.results)}
	for i, m := range sess.results {
		view.entries = append(view.entries, listEntry{
			text:    Lines(pickEntry(i+1, m, look)),
			buttons: []Button{{Label: fmt.Sprint(i + 1), Data: data(sess.id, actionMedia, i)}},
			poster:  m.PosterURL,
		})
	}
	return view
}

// notice is failure for a step that keeps the conversation as it was.
func (e *Engine) notice(step string, err error) Reply {
	if msg, ok := UserMessage(err); ok {
		return Reply{Notice: "⚠️ " + msg}
	}
	e.log.Error("backend call failed", "step", step, "err", err)
	return Reply{Notice: msgBackendDown}
}
