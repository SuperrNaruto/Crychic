package flow

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"
)

const (
	actionChart     = "d" // arg: index into charts; shows its first page
	actionChartPage = "g" // arg: page of the current chart, from 0
	actionChartPick = "i" // arg: index into the chart's picks
	actionCharts    = "b" // back to the list of charts
	actionWeekday   = "w" // arg: calendar weekday, 1 Monday … 7 Sunday

	// chartPageSize is how many picks one page shows as buttons.
	chartPageSize = 8
	// gridColumns is how many number buttons share a row.
	gridColumns = 4
	// menuColumns is how many menu entries share a row.
	menuColumns = 2
	// chartOverviewRunes keeps a full page of synopses inside one message.
	chartOverviewRunes = 300

	msgCharts      = "🔥 发现"
	msgPickChart   = "选择一个榜单："
	msgNoExactPick = "没有对应的条目？可以用 /request 换个名字搜。"
	msgEmptyChart  = "这个榜单暂时是空的。"
	msgEmptyDay    = "这天没有新番放送。"
	msgDetailPage  = "查看详情页"

	daysInWeek = 7
	// calendarOffset is the time zone "today" is taken in: the bot's users
	// and Bangumi's weekdays are on China time.
	calendarOffset = 8 * 60 * 60
)

// calendarZone is the zone of calendarOffset.
var calendarZone = time.FixedZone("UTC+8", calendarOffset)

// weekdayNames are the calendar weekdays, Monday first.
var weekdayNames = [daysInWeek]string{"一", "二", "三", "四", "五", "六", "日"}

// chart is one entry of the discover menu.
type chart struct {
	kind  Chart
	icon  string // heads the chart's pages; buttons go without
	label string
	aired bool // a weekly calendar: by weekday, then first air date; links and synopses looked up
}

var charts = []chart{
	{kind: Trending, icon: "🔥", label: "TMDB 流行趋势"},
	{kind: HotMovies, icon: "🎬", label: "豆瓣热门电影"},
	{kind: HotShows, icon: "📺", label: "豆瓣热门剧集"},
	{kind: InTheaters, icon: "🎟️", label: "正在热映"},
	{kind: NewAnime, icon: "🎌", label: "新番放送", aired: true},
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
		return e.chartPage(ctx, sess, p.arg), true
	case actionWeekday:
		if p.arg < 1 || p.arg > daysInWeek {
			return Reply{Notice: msgInvalidChoice}, true
		}
		sess.day = p.arg
		return e.chartPage(ctx, sess, 0), true
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
	sess := e.store.create(actor, nil)
	return e.shown(sess.id, e.chartMenu(sess))
}

// chartMenu lists the charts to discover from.
func (e *Engine) chartMenu(sess session) Reply {
	e.store.put(sess)
	buttons := make([]Button, 0, len(charts))
	for i, c := range charts {
		buttons = append(buttons, Button{Label: c.label, Data: data(sess.id, actionChart, i)})
	}
	rows := append(grid(buttons, menuColumns), []Button{homeButton(sess.id)})
	return Reply{Text: Lines(Heading(Plain(msgCharts)), Line(Plain(msgPickChart))), Buttons: rows}
}

// openChart reads chart index afresh; a calendar opens on today.
func (e *Engine) openChart(ctx context.Context, sess session, index int) Reply {
	picks, err := e.chartPicks(ctx, charts[index])
	if err != nil {
		e.store.take(sess.id)
		return e.failure("discover", err)
	}
	sess.chart, sess.picks, sess.noted, sess.day = index, picks, nil, 0
	if charts[index].aired {
		sess.day = e.today()
	}
	return e.chartPage(ctx, sess, 0)
}

// chartPicks lists c's picks; a calendar's are ordered by weekday, then
// first air date, unknown dates last.
func (e *Engine) chartPicks(ctx context.Context, c chart) ([]Media, error) {
	if !c.aired {
		return e.backend.Discover(ctx, c.kind)
	}
	picks, err := e.calendar.Calendar(ctx)
	slices.SortStableFunc(picks, func(a, b Media) int {
		return cmp.Or(cmp.Compare(a.Weekday, b.Weekday), cmp.Compare(airKey(a), airKey(b)))
	})
	return picks, err
}

// airKey orders by first air date, unknown dates last.
func airKey(m Media) string {
	if m.Released == "" {
		return "~"
	}
	return m.Released
}

// today is the calendar weekday now, 1 Monday … 7 Sunday.
func (e *Engine) today() int {
	if d := int(e.now().In(calendarZone).Weekday()); d != int(time.Sunday) {
		return d
	}
	return daysInWeek
}

// span is the run of picks a page shows: [first, end), numbered from
// first-base+1.
type span struct{ first, end, base int }

// view is the run of picks being browsed: a calendar's weekday, else all.
func (sess session) view() (lo, hi int) {
	if sess.day == 0 {
		return 0, len(sess.picks)
	}
	lo = slices.IndexFunc(sess.picks, func(m Media) bool { return m.Weekday == sess.day })
	if lo < 0 {
		return 0, 0
	}
	hi = lo
	for hi < len(sess.picks) && sess.picks[hi].Weekday == sess.day {
		hi++
	}
	return lo, hi
}

// chartPage shows page of the picks in view, numbered, with a grid of
// number buttons to pick one. Calendar picks are annotated first.
func (e *Engine) chartPage(ctx context.Context, sess session, page int) Reply {
	lo, hi := sess.view()
	pages := max((hi-lo+chartPageSize-1)/chartPageSize, 1)
	if page < 0 || page >= pages {
		return Reply{Notice: msgInvalidChoice}
	}
	c := charts[sess.chart]
	at := span{first: lo + page*chartPageSize, base: lo}
	at.end = min(at.first+chartPageSize, hi)
	if c.aired {
		e.annotate(ctx, &sess, at.first, at.end)
	}
	sess.page = page
	e.store.put(sess)
	text := Lines(Heading(Plain(fmt.Sprintf("%s %s%s", c.icon, c.label, e.dayName(sess.day)))))
	text = append(text, pickLines(at, sess.picks[at.first:at.end], c.aired)...)
	if hi == lo {
		text = append(text, Line(Plain(emptyNote(c))))
	}
	if pages > 1 {
		text = append(text, Remark(fmt.Sprintf("第 %d/%d 页", page+1, pages)))
	}
	rows := numberGrid(sess.id, at)
	if nav := pager(sess.id, page, pages); len(nav) > 0 {
		rows = append(rows, nav)
	}
	if c.aired {
		rows = append(rows, weekdayRow(sess.id, sess.day))
	}
	rows = append(rows, []Button{{Label: "返回榜单", Data: data(sess.id, actionCharts, 0)}, homeButton(sess.id)})
	return Reply{Text: text, Buttons: rows}
}

func emptyNote(c chart) string {
	if c.aired {
		return msgEmptyDay
	}
	return msgEmptyChart
}

// dayName is " · 星期一（今天）" for a calendar weekday, "" otherwise.
func (e *Engine) dayName(day int) string {
	if day == 0 {
		return ""
	}
	name := " · 星期" + weekdayNames[day-1]
	if day == e.today() {
		name += "（今天）"
	}
	return name
}

// weekdayRow switches the calendar to another weekday; the shown one is marked.
func weekdayRow(id uint64, shown int) []Button {
	row := make([]Button, 0, daysInWeek)
	for i, name := range weekdayNames {
		if i+1 == shown {
			name = "·" + name + "·"
		}
		row = append(row, Button{Label: name, Data: data(id, actionWeekday, i+1)})
	}
	return row
}

// pickLines lists picks numbered from at.first-at.base+1, each opening to
// its synopsis (Douban's is a year / region / genre / cast line). Calendars
// group them under their air date, which stands in for the year; the kind
// is only named when the page mixes movies and shows.
func pickLines(at span, picks []Media, aired bool) Text {
	mixed := mixedKinds(picks)
	var text Text
	day := ""
	for i, m := range picks {
		if aired && m.Released != day {
			day = m.Released
			text = append(text, Group(airDay(day)))
		}
		text = append(text, pickEntry(at.first-at.base+i+1, m, pickLook{year: !aired, kind: mixed}))
	}
	return text
}

// pickLook says which metadata a chart line repeats.
type pickLook struct{ year, kind bool }

// pickEntry is a short chart entry on one line, e.g. "1. **沙丘** · _2021 · ⭐ 7.8_";
// the original title waits for the card. A pick with a synopsis opens by
// its name to show it, with the link to the media's page below; one
// without has its name linked instead.
func pickEntry(n int, m Media, look pickLook) Block {
	var y, kind string
	if look.year {
		y = m.Year
	}
	if look.kind {
		kind = m.Kind.String()
	}
	item := entry(n, Linked(Strong(m.Title), m.Link), y, kind, rating(m.Rating))
	item.Brief = true
	if m.Overview == "" {
		return item
	}
	item.Spans = []Span{Strong(m.Title)}
	item.Body = Lines(Line(Plain(truncate(m.Overview, chartOverviewRunes))))
	if m.Link != "" {
		item.Body = append(item.Body, Line(), Line(Linked(Plain(msgDetailPage), m.Link)))
	}
	return item
}

func airDay(day string) string {
	if day == "" {
		return "首播日期未定"
	}
	return day + " 首播"
}

func mixedKinds(picks []Media) bool {
	for _, m := range picks {
		if m.Kind != picks[0].Kind {
			return true
		}
	}
	return false
}

// numberGrid is the pick buttons, numbered like the lines, gridColumns a row.
func numberGrid(id uint64, at span) [][]Button {
	picks := make([]Button, 0, at.end-at.first)
	for i := at.first; i < at.end; i++ {
		picks = append(picks, Button{Label: fmt.Sprint(i - at.base + 1), Data: data(id, actionChartPick, i)})
	}
	return grid(picks, gridColumns)
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
	reply := e.resultList(sess, pick.Title)
	reply.Text = append(reply.Text, Remark(msgNoExactPick))
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
