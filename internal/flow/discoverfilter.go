package flow

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	actionFilter      = "ya" // opens the condition search
	actionFilterKind  = "yk" // arg: Kind to search
	actionFilterField = "yf" // arg: field whose options to show
	actionFilterSet   = "yo" // arg: field*optionStride + option
	actionFilterRun   = "ys" // searches with the conditions
	actionFilterMore  = "ym" // reads the next upstream page

	// optionStride packs a field and one of its options into one argument.
	optionStride = 100
	// tmdbPageSize is how many results a full TMDB discover page holds; a
	// shorter page is the last.
	tmdbPageSize = 20
	// maxFound bounds the results gathered over several pages.
	maxFound = 100
	// filterColumns is how many option or field buttons share a row.
	filterColumns = 3
	ratingBits    = 64

	msgFilterTitle = "🎯 条件找片"
	msgFilterHint  = "按 TMDB 的数据找片，选好条件点「开始找片」～"
	msgFilterFound = "🎯 找到的作品"
	msgFilterNone  = "没有符合条件的作品，放宽一点条件试试吧～"
	msgFilterLast  = "已经是全部结果啦～"
	labelFilterRun = "开始找片"
	labelMore      = "更多结果"
)

// Condition fields, in the order the panel shows them.
const (
	fieldGenre = iota
	fieldLanguage
	fieldRating
	fieldVotes
	fieldSince
	fieldSort
	filterFields
)

// filterOption is one choice of a field: a label and the value sent.
type filterOption struct{ label, value string }

var fieldNames = [filterFields]string{"题材", "语言", "评分", "评分人数", "上映", "排序"}

// Genre ids and languages follow MoviePilot v3.1.4's TMDB discover page.
var movieGenres = []filterOption{
	{"不限", ""}, {"动作", "28"}, {"冒险", "12"}, {"动画", "16"}, {"喜剧", "35"}, {"犯罪", "80"},
	{"纪录", "99"}, {"剧情", "18"}, {"家庭", "10751"}, {"奇幻", "14"}, {"历史", "36"}, {"恐怖", "27"},
	{"音乐", "10402"}, {"悬疑", "9648"}, {"爱情", "10749"}, {"科幻", "878"}, {"电视电影", "10770"},
	{"惊悚", "53"}, {"战争", "10752"}, {"西部", "37"},
}

var tvGenres = []filterOption{
	{"不限", ""}, {"动作冒险", "10759"}, {"动画", "16"}, {"喜剧", "35"}, {"犯罪", "80"}, {"纪录", "99"},
	{"剧情", "18"}, {"家庭", "10751"}, {"儿童", "10762"}, {"悬疑", "9648"}, {"新闻", "10763"},
	{"真人秀", "10764"}, {"科幻奇幻", "10765"}, {"肥皂剧", "10766"}, {"脱口秀", "10767"},
	{"战争政治", "10768"}, {"西部", "37"},
}

var languages = []filterOption{
	{"不限", ""}, {"中文", "zh"}, {"英语", "en"}, {"日语", "ja"}, {"韩语", "ko"}, {"法语", "fr"},
	{"德语", "de"}, {"西班牙语", "es"}, {"意大利语", "it"}, {"俄语", "ru"}, {"葡萄牙语", "pt"},
	{"阿拉伯语", "ar"}, {"印地语", "hi"}, {"泰语", "th"},
}

var ratings = []filterOption{{"不限", "0"}, {"≥6", "6"}, {"≥7", "7"}, {"≥8", "8"}}

// votes start at MoviePilot's own default of 10, which keeps barely rated
// titles from topping a sort by rating.
var votes = []filterOption{{"≥10", "10"}, {"不限", "0"}, {"≥100", "100"}, {"≥1000", "1000"}}

// since values are years back from today.
var since = []filterOption{{"不限", ""}, {"近 1 年", "1"}, {"近 3 年", "3"}, {"近 10 年", "10"}}

var movieSorts = []filterOption{{"热门", "popularity.desc"}, {"评分最高", "vote_average.desc"}, {"最新", "release_date.desc"}}

var tvSorts = []filterOption{{"热门", "popularity.desc"}, {"评分最高", "vote_average.desc"}, {"最新", "first_air_date.desc"}}

// discoverFilter is the conditions being edited, by option index; a value
// so that history snapshots keep their own copy.
type discoverFilter struct {
	kind   Kind
	choice [filterFields]int
	open   int  // field whose options show, plus one; 0 when none
	page   int  // upstream pages read for the shown results
	more   bool // the last page read was full
}

// options are field's choices for kind.
func options(field int, kind Kind) []filterOption {
	switch field {
	case fieldGenre:
		return byKind(kind, movieGenres, tvGenres)
	case fieldLanguage:
		return languages
	case fieldRating:
		return ratings
	case fieldVotes:
		return votes
	case fieldSince:
		return since
	}
	return byKind(kind, movieSorts, tvSorts)
}

func byKind(kind Kind, movie, tv []filterOption) []filterOption {
	if kind == TV {
		return tv
	}
	return movie
}

// chooseFilter applies the condition search actions; ok is false for others.
func (e *Engine) chooseFilter(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionFilter:
		if sess.filter.kind == 0 {
			sess.filter = discoverFilter{kind: Movie}
		}
		sess.filter.open = 0
		return e.filterPanel(sess), true
	case actionFilterKind:
		return e.setFilterKind(sess, Kind(p.arg)), true
	case actionFilterField:
		return e.openFilterField(sess, p.arg), true
	case actionFilterSet:
		return e.setFilterOption(sess, p.arg/optionStride, p.arg%optionStride), true
	case actionFilterRun:
		return e.runFilter(ctx, sess), true
	case actionFilterMore:
		return e.moreFound(ctx, sess), true
	}
	return Reply{}, false
}

func (e *Engine) setFilterKind(sess session, kind Kind) Reply {
	if kind != Movie && kind != TV || sess.filter.kind == 0 {
		return Reply{Notice: msgInvalidChoice}
	}
	if kind != sess.filter.kind {
		// Genre ids differ by kind; sorts keep their meaning.
		sess.filter.kind, sess.filter.choice[fieldGenre] = kind, 0
	}
	return e.filterPanel(sess)
}

func (e *Engine) openFilterField(sess session, field int) Reply {
	if field < 0 || field >= filterFields || sess.filter.kind == 0 {
		return Reply{Notice: msgInvalidChoice}
	}
	sess.filter.open = field + 1
	return e.filterPanel(sess)
}

func (e *Engine) setFilterOption(sess session, field, option int) Reply {
	if field < 0 || field >= filterFields || sess.filter.kind == 0 || option < 0 || option >= len(options(field, sess.filter.kind)) {
		return Reply{Notice: msgInvalidChoice}
	}
	sess.filter.choice[field], sess.filter.open = option, 0
	return e.filterPanel(sess)
}

// filterPanel shows the conditions, the open field's options above the
// kind switch, the fields to change, and 开始找片.
func (e *Engine) filterPanel(sess session) Reply {
	e.store.put(sess)
	f := sess.filter
	rows := [][]Button{}
	if f.open > 0 {
		field := f.open - 1
		var choices []Button
		for i, o := range options(field, f.kind) {
			choices = append(choices, Button{Label: markShown(o.label, i == f.choice[field]), Data: data(sess.id, actionFilterSet, field*optionStride+i)})
		}
		rows = append(rows, grid(choices, filterColumns)...)
	}
	rows = append(rows, []Button{
		{Label: markShown(Movie.String(), f.kind == Movie), Data: data(sess.id, actionFilterKind, int(Movie))},
		{Label: markShown(TV.String(), f.kind == TV), Data: data(sess.id, actionFilterKind, int(TV))},
	})
	fields := make([]Button, 0, filterFields)
	for field, name := range fieldNames {
		fields = append(fields, Button{Label: markShown(name, f.open == field+1), Data: data(sess.id, actionFilterField, field)})
	}
	rows = append(rows, grid(fields, filterColumns)...)
	rows = append(rows, []Button{{Label: labelFilterRun, Data: data(sess.id, actionFilterRun, 0)}}, browseRow(sess.id))
	text := Lines(Heading(Plain(msgFilterTitle)), Line(Plain(msgFilterHint)), Table(filterHead, f.rows()...))
	if f.open > 0 {
		text = append(text, Remark(fmt.Sprintf("选一个「%s」：", fieldNames[f.open-1])))
	}
	return Reply{Text: text, Buttons: rows}
}

var filterHead = []string{"条件", "设置"}

// rows are the kind and each field's chosen option, one table row each.
func (f discoverFilter) rows() [][]Span {
	rows := [][]Span{{Plain("类型"), Plain(f.kind.String())}}
	for field, name := range fieldNames {
		rows = append(rows, []Span{Plain(name), Plain(options(field, f.kind)[f.choice[field]].label)})
	}
	return rows
}

// summary is the chosen conditions on one line, e.g. 电视剧 · 动画 · 日语.
func (f discoverFilter) summary() string {
	parts := []string{f.kind.String()}
	for field := range filterFields {
		if o := options(field, f.kind)[f.choice[field]]; o.label != "不限" {
			parts = append(parts, o.label)
		}
	}
	return strings.Join(parts, " · ")
}

// query is the TMDB search for the conditions and upstream page.
func (e *Engine) query(f discoverFilter, page int) DiscoverQuery {
	value := func(field int) string { return options(field, f.kind)[f.choice[field]].value }
	q := DiscoverQuery{Kind: f.kind, Genre: value(fieldGenre), Language: value(fieldLanguage), Sort: value(fieldSort), Page: page}
	q.MinRating, _ = strconv.ParseFloat(value(fieldRating), ratingBits)
	q.MinVotes, _ = strconv.Atoi(value(fieldVotes))
	if years, err := strconv.Atoi(value(fieldSince)); err == nil {
		q.Since = e.now().In(calendarZone).AddDate(-years, 0, 0).Format(dateLayout)
	}
	return q
}

// runFilter reads the first page of results. Nothing found only flashes a
// notice, so the conditions stay on screen to loosen.
func (e *Engine) runFilter(ctx context.Context, sess session) Reply {
	if sess.filter.kind == 0 {
		return Reply{Notice: msgInvalidChoice}
	}
	found, err := e.backend.DiscoverBy(ctx, e.query(sess.filter, 1))
	if err != nil {
		return e.notice("discover by", err)
	}
	if len(found) == 0 {
		return Reply{Notice: msgFilterNone}
	}
	sess.picks = found[:min(len(found), maxFound)]
	sess.filter.open, sess.filter.page, sess.filter.more = 0, 1, len(found) >= tmdbPageSize
	return e.listPages(sess, foundView(sess))
}

// moreFound reads the next upstream page and shows the page holding the
// first new result; results already listed keep their numbers.
func (e *Engine) moreFound(ctx context.Context, sess session) Reply {
	if !sess.filter.more || len(sess.picks) >= maxFound {
		return Reply{Notice: msgFilterLast}
	}
	found, err := e.backend.DiscoverBy(ctx, e.query(sess.filter, sess.filter.page+1))
	if err != nil {
		return e.notice("discover by", err)
	}
	first := len(sess.picks)
	picks := slices.Clone(sess.picks)
	for _, m := range found {
		if len(picks) < maxFound && !slices.ContainsFunc(picks, func(p Media) bool { return p.Source == m.Source && p.ID == m.ID }) {
			picks = append(picks, m)
		}
	}
	sess.picks, sess.filter.page = picks, sess.filter.page+1
	sess.filter.more = len(found) >= tmdbPageSize && len(picks) < maxFound
	if len(picks) == first {
		e.store.put(sess)
		return Reply{Notice: msgFilterLast}
	}
	return e.listPageWith(sess, foundView(sess), data(sess.id, actionWorkPick, first))
}

// foundView lists the results like person works, with the conditions as a
// note and 更多结果 while TMDB may have more.
func foundView(sess session) listView {
	view := pickView(sess, msgFilterFound)
	view.note = "条件：" + sess.filter.summary()
	if sess.filter.more {
		view.menu = [][]Button{{{Label: labelMore, Data: data(sess.id, actionFilterMore, 0)}}}
	}
	return view
}
