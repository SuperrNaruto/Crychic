package flow

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

const (
	searchPageItems = 8
	actionResearch  = "rs" // asks for a new title, replacing this conversation
)

// Start searches for a title. A unique result opens directly but never
// bypasses the final subscription confirmation.
func (e *Engine) Start(ctx context.Context, actor Actor, term string) Reply {
	term = strings.TrimSpace(term)
	if term == "" {
		return Reply{Text: Sentence(msgUsage)}
	}
	sess := e.store.create(actor, nil)
	return e.shown(sess.id, e.search(ctx, sess, term))
}

func (e *Engine) search(ctx context.Context, sess session, term string) Reply {
	sess.query = term
	results, err := e.backend.Search(ctx, term)
	if err != nil {
		return e.readFailure(sess, recovery{step: readSearch}, err)
	}
	sess.results, sess.retry = results, recovery{}
	e.store.put(sess)
	if len(results) == 0 {
		reply := askResearch(sess)
		reply.Text = Lines(
			Heading(Plain("🔍 没有找到相关影视")),
			Line(Plain(fmt.Sprintf("没有找到「%s」相关的影视，请直接回复新的片名。", term))),
		)
		return reply
	}
	if len(results) == 1 {
		reply := e.pickMedia(ctx, sess, 0)
		if _, live := e.store.get(sess.id); live {
			reply = withResearch(reply, sess.id)
		}
		return reply
	}
	return e.resultList(sess, term)
}

// resultList keeps every returned result reachable, with stable numbers
// and the same content budget as other lists.
func (e *Engine) resultList(sess session, term string) Reply {
	view := listView{
		heading:   Heading(Plain(fmt.Sprintf("🔍「%s」的搜索结果", term))),
		pageItems: searchPageItems,
		footer:    []Button{researchButton(sess.id), cancelButton(sess.id)},
	}
	for i, m := range sess.results {
		m.Title = truncate(m.Title, listTitleRunes)
		m.OriginalTitle = truncate(m.OriginalTitle, listTitleRunes)
		view.entries = append(view.entries, listEntry{
			text:    Lines(resultEntry(i+1, m)),
			buttons: []Button{{Label: fmt.Sprint(i + 1), Data: data(sess.id, actionMedia, i)}},
			poster:  m.PosterURL,
		})
	}
	return e.listPages(sess, view)
}

func researchButton(id uint64) Button {
	return Button{Label: "重新搜索", Data: data(id, actionResearch, 0)}
}

func withResearch(reply Reply, id uint64) Reply {
	if reply.Input != "" || reply.Notice != "" {
		return reply
	}
	button := researchButton(id)
	for _, row := range reply.Buttons {
		if slices.Contains(row, button) {
			return reply
		}
	}
	reply.Buttons = append(slices.Clone(reply.Buttons), []Button{button})
	return reply
}

func askResearch(sess session) Reply {
	return Reply{
		Text:    Lines(Heading(Plain("🔍 重新搜索")), Line(Plain(msgAskTitle))),
		Buttons: [][]Button{{cancelButton(sess.id)}},
		Input:   data(sess.id, actionResearch, 0),
	}
}

// A replacement search uses a fresh session, so buttons from the previous
// title cannot select or submit a different title by their old index.
func (e *Engine) research(ctx context.Context, sess session, text string) Reply {
	if text == "" {
		return e.navigate(sess, answered, askResearch(sess))
	}
	e.store.take(sess.id)
	return e.Start(ctx, sess.owner, text)
}
