package flow

import (
	"context"
	"fmt"
)

const (
	actionChart     = "d" // arg: index into charts; shows its first page
	actionChartPage = "g" // arg: page of the current chart, from 0
	actionChartPick = "i" // arg: index into the chart's picks
	actionCharts    = "b" // back to the list of charts

	// chartPageSize is how many picks one page shows as buttons.
	chartPageSize = 8

	msgCharts      = "🔥 发现"
	msgPickChart   = "选择一个榜单："
	msgNoExactPick = "没有对应的条目？可以用 /request 换个名字搜。"
)

// chart is one entry of the discover menu.
type chart struct {
	kind  Chart
	label string
	aired bool // show first air dates, for release calendars
}

var charts = []chart{
	{kind: Trending, label: "🔥 TMDB 流行趋势"},
	{kind: HotMovies, label: "🎬 豆瓣热门电影"},
	{kind: HotShows, label: "📺 豆瓣热门剧集"},
	{kind: InTheaters, label: "🎟️ 正在热映"},
	{kind: NewAnime, label: "🎌 新番放送", aired: true},
}

// chooseChart applies the discover actions; ok is false for others.
func (e *Engine) chooseChart(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionCharts:
		return e.chartMenu(sess), true
	case actionChart:
		if p.arg < 0 || p.arg >= len(charts) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		return e.openChart(ctx, sess, p.arg), true
	case actionChartPage:
		return e.chartPage(sess, p.arg), true
	case actionChartPick:
		if p.arg < 0 || p.arg >= len(sess.picks) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		return e.pickFromChart(ctx, sess, sess.picks[p.arg]), true
	}
	return Reply{}, false
}

// Charts opens the discover menu.
func (e *Engine) Charts(_ context.Context, actor Actor) Reply {
	return e.chartMenu(e.store.create(actor, nil))
}

// chartMenu lists the charts to discover from.
func (e *Engine) chartMenu(sess session) Reply {
	e.store.put(sess)
	rows := make([][]Button, 0, len(charts)+1)
	for i, c := range charts {
		rows = append(rows, []Button{{Label: c.label, Data: data(sess.id, actionChart, i)}})
	}
	rows = append(rows, []Button{homeButton(sess.id)})
	return Reply{Text: Lines(Line(Strong(msgCharts)), Line(Plain(msgPickChart))), Buttons: rows}
}

func (e *Engine) openChart(ctx context.Context, sess session, index int) Reply {
	picks, err := e.backend.Discover(ctx, charts[index].kind)
	if err != nil {
		e.store.take(sess.id)
		return e.failure("discover", err)
	}
	sess.chart, sess.picks = index, picks
	return e.chartPage(sess, 0)
}

// chartPage shows page of the current chart's picks as numbered buttons.
func (e *Engine) chartPage(sess session, page int) Reply {
	pages := max((len(sess.picks)+chartPageSize-1)/chartPageSize, 1)
	if page < 0 || page >= pages {
		return Reply{Notice: msgInvalidChoice}
	}
	e.store.put(sess)
	c := charts[sess.chart]
	text := Lines(Line(Strong(fmt.Sprintf("%s · 第 %d/%d 页", c.label, page+1, pages))))
	var rows [][]Button
	first := page * chartPageSize
	for i := first; i < min(first+chartPageSize, len(sess.picks)); i++ {
		m := sess.picks[i]
		text = append(text, pickLine(i+1, m, c.aired))
		rows = append(rows, []Button{{Label: fmt.Sprintf("%d. %s", i+1, titleYear(m)), Data: data(sess.id, actionChartPick, i)}})
	}
	if len(sess.picks) == 0 {
		text = append(text, Line(Plain("这个榜单暂时是空的。")))
	}
	if nav := pager(sess.id, page, pages); len(nav) > 0 {
		rows = append(rows, nav)
	}
	rows = append(rows, []Button{{Label: "返回榜单", Data: data(sess.id, actionCharts, 0)}, homeButton(sess.id)})
	return Reply{Text: text, Buttons: rows}
}

// pickLine is a result line, with the first air date for calendars.
func pickLine(n int, m Media, aired bool) Block {
	line := resultLine(n, m)
	if aired && m.Released != "" {
		line.Spans = append(line.Spans, Plain(" · "+m.Released+" 首播"))
	}
	return line
}

// pager is the previous/next row; a missing direction is left out.
func pager(id uint64, page, pages int) []Button {
	var row []Button
	if page > 0 {
		row = append(row, Button{Label: "‹ 上一页", Data: data(id, actionChartPage, page-1)})
	}
	if page < pages-1 {
		row = append(row, Button{Label: "下一页 ›", Data: data(id, actionChartPage, page+1)})
	}
	return row
}

// pickFromChart turns a chart pick into a subscribable search result. Picks
// may come from Douban or Bangumi, whose ids MoviePilot's transfers never
// carry, so the pick is looked up by title: a sure match goes straight on,
// otherwise the user chooses among the search results.
func (e *Engine) pickFromChart(ctx context.Context, sess session, pick Media) Reply {
	results, err := e.backend.Search(ctx, pick.Title)
	if err == nil && len(results) == 0 && pick.OriginalTitle != "" && pick.OriginalTitle != pick.Title {
		results, err = e.backend.Search(ctx, pick.OriginalTitle)
	}
	if err != nil {
		e.store.take(sess.id)
		return e.failure("search", err)
	}
	if len(results) == 0 {
		e.store.take(sess.id)
		return Reply{Text: Sentence(fmt.Sprintf("🔍 没有找到「%s」的可订阅条目，可以用 /request 换个名字搜。", pick.Title))}
	}
	sess.results = results[:min(len(results), MaxResults)]
	if i, ok := sameMedia(sess.results, pick); ok {
		return e.pickMedia(ctx, sess, i)
	}
	e.store.put(sess)
	reply := resultList(sess, pick.Title)
	reply.Text = append(reply.Text, Line(Emphasis(msgNoExactPick)))
	return reply
}

// sameMedia finds pick among results: by identity, else the one result
// with its title, year and kind.
func sameMedia(results []Media, pick Media) (int, bool) {
	found := -1
	for i, r := range results {
		if r.Source == pick.Source && r.ID == pick.ID {
			return i, true
		}
		if r.Title != pick.Title || r.Year != pick.Year || r.Kind != pick.Kind {
			continue
		}
		if found >= 0 {
			return 0, false
		}
		found = i
	}
	return found, found >= 0
}
