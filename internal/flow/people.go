package flow

import (
	"context"
	"fmt"
)

const (
	actionCast       = "ca" // lists the people credited on the picked media
	actionCastPerson = "cw" // arg: index into the cast; lists that person's works
	actionWorkPick   = "cp" // arg: index into the listed works; picks one like a chart pick

	// maxCredits and maxWorks bound the lists; longer ones still page.
	maxCredits = 40
	maxWorks   = 60

	bangumiSource = "bangumi"
	labelCast     = "演职人员"
	msgNoCast     = "没找到这部作品的演职人员呢～"
	msgNoWorks    = "没找到这位的其他作品呢～"
)

// castSource reports whether media's source credits people here: TMDB
// lists the cast, Bangumi the voice actors.
func castSource(media Media) bool {
	return media.Source == tmdbSource || media.Source == bangumiSource
}

// choosePeople applies the cast and works actions; ok is false for others.
func (e *Engine) choosePeople(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionCast:
		return e.castList(ctx, sess), true
	case actionCastPerson:
		if p.arg < 0 || p.arg >= len(sess.people) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		return e.personWorks(ctx, sess, sess.people[p.arg]), true
	case actionWorkPick:
		if p.arg < 0 || p.arg >= len(sess.picks) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		return e.pickFromChart(ctx, sess, sess.picks[p.arg]), true
	}
	return Reply{}, false
}

// castList lists the people credited on the picked media. Browsing is
// optional, so nothing found or a failed read only flashes a notice and
// the card stays.
func (e *Engine) castList(ctx context.Context, sess session) Reply {
	media := sess.picked.Media
	if media.ID == "" || !castSource(media) {
		return Reply{Notice: msgInvalidChoice}
	}
	people, err := e.backend.Cast(ctx, media)
	if err != nil {
		return e.notice("cast", err)
	}
	if len(people) == 0 {
		return Reply{Notice: msgNoCast}
	}
	sess.people = people[:min(len(people), maxCredits)]
	view := listView{heading: Heading(Plain(fmt.Sprintf("🎭「%s」的演职人员", media.Title))), footer: browseRow(sess.id)}
	for i, p := range sess.people {
		view.entries = append(view.entries, listEntry{
			text:    Lines(entry(i+1, Strong(p.Name), truncate(p.Role, listTitleRunes))),
			buttons: []Button{{Label: fmt.Sprint(i + 1), Data: data(sess.id, actionCastPerson, i)}},
		})
	}
	return e.listPages(sess, view)
}

// personWorks lists the media p is credited on, each picked like a chart
// pick: TMDB works by their identity, others searched by title.
func (e *Engine) personWorks(ctx context.Context, sess session, p Person) Reply {
	works, err := e.backend.PersonWorks(ctx, p)
	if err != nil {
		return e.notice("person works", err)
	}
	if len(works) == 0 {
		return Reply{Notice: msgNoWorks}
	}
	sess.picks, sess.download = works[:min(len(works), maxWorks)], false
	return e.listPages(sess, pickView(sess, fmt.Sprintf("🎭 %s 的作品", p.Name)))
}

// pickView lists sess.picks like a chart: posters in a gallery, each entry
// opens to its synopsis, a number button picks it.
func pickView(sess session, heading string) listView {
	view := listView{heading: Heading(Plain(heading)), footer: browseRow(sess.id)}
	look := pickLook{year: true, kind: mixedKinds(sess.picks)}
	for i, m := range sess.picks {
		view.entries = append(view.entries, listEntry{
			text:    Lines(pickEntry(i+1, m, look)),
			buttons: []Button{{Label: fmt.Sprint(i + 1), Data: data(sess.id, actionWorkPick, i)}},
			poster:  m.PosterURL,
		})
	}
	return view
}
