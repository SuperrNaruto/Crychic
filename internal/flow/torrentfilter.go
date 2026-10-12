package flow

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
)

const (
	actionTorrentFilter      = "ti" // shows the combined release filter
	actionTorrentFilterField = "tj" // arg: index into releaseFields; offers its values
	actionTorrentFilterTick  = "tl" // arg: the offer's generation × filterOfferBase + index into its values; ticks or unticks it
	actionTorrentFilterClear = "tn" // arg: 0 clears every field, else 1 + the field to clear

	fieldColumns = 3
	// filterOfferBase exceeds any number of offered values, so a tick's arg
	// carries the generation of the field opening it was offered by.
	filterOfferBase = 100000
	bytesPerGiB     = 1 << 30

	msgReleaseFilter = "组合筛选"
	msgAnyValue      = "不限"
	msgClearFilters  = "清除全部条件"
	msgUnknownValue  = "未知"
	msgFilterPick    = "选好后点「返回」看结果～"
	msgFilterTicks   = "可以多选，再点一次取消；" + msgFilterPick
	msgFilterFields  = "点条件名选择，" + msgFilterPick
)

// releaseField is one fact releases can be narrowed by. value is "" when
// the release does not say; such releases only show while the field is
// not narrowed or 未知 is ticked.
type releaseField struct {
	label string
	value func(Torrent) string
	bands []string // a fixed order of values; nil orders by count
}

// sizeBands bound the 大小 values, in GiB.
var sizeBands = []struct {
	below float64
	label string
}{{2, "2 GiB 以下"}, {10, "2–10 GiB"}, {30, "10–30 GiB"}, {60, "30–60 GiB"}}

const sizeTop = "60 GiB 以上"

var releaseFields = []releaseField{
	{label: "分辨率", value: func(t Torrent) string { return t.Resolution }},
	{label: "编码", value: func(t Torrent) string { return t.Video }},
	{label: "制作组", value: func(t Torrent) string { return t.Group }},
	{label: "大小", value: sizeBand, bands: sizeLabels()},
	{label: "促销", value: promotionOf},
	{label: "H&R", value: hitAndRunValue},
}

func hitAndRunValue(t Torrent) string {
	if t.HitAndRun {
		return "H&R"
	}
	return "无 H&R"
}

func sizeBand(t Torrent) string {
	if t.Size <= 0 {
		return ""
	}
	for _, band := range sizeBands {
		if t.Size < band.below*bytesPerGiB {
			return band.label
		}
	}
	return sizeTop
}

func sizeLabels() []string {
	var labels []string
	for _, band := range sizeBands {
		labels = append(labels, band.label)
	}
	return append(labels, sizeTop)
}

// promotionOf is a release's promotion as MoviePilot words it, "" when it
// says it does not know.
func promotionOf(t Torrent) string {
	if t.Promotion == msgUnknownValue {
		return ""
	}
	return t.Promotion
}

// releaseFilter narrows the listed releases: a field with ticked values
// keeps releases having one of them, and every narrowed field must agree.
// It is a value; changes replace its slices so history snapshots keep
// their own.
type releaseFilter struct {
	picked  [][]string // by field; nil leaves the field open
	open    int        // field whose values show, plus one; 0 when none
	offered []string   // the open field's values as offered, which ticks index
	offer   int        // generation of offered; a tick from another opening is stale
}

func (f releaseFilter) active() bool {
	return slices.ContainsFunc(f.picked, func(values []string) bool { return len(values) > 0 })
}

func (f releaseFilter) ticked(field int) []string {
	if field < len(f.picked) {
		return f.picked[field]
	}
	return nil
}

// passes reports whether t agrees with every narrowed field but skip.
func (f releaseFilter) passes(t Torrent, skip int) bool {
	for i, field := range releaseFields {
		if values := f.ticked(i); i != skip && len(values) > 0 && !slices.Contains(values, field.value(t)) {
			return false
		}
	}
	return true
}

// summary is e.g. "分辨率 2160p/1080p · 促销 免费".
func (f releaseFilter) summary() string {
	var parts []string
	for i, field := range releaseFields {
		if values := f.ticked(i); len(values) > 0 {
			parts = append(parts, field.label+" "+strings.Join(valueLabels(values), "/"))
		}
	}
	return strings.Join(parts, " · ")
}

func valueLabels(values []string) []string {
	labels := make([]string, len(values))
	for i, v := range values {
		labels[i] = cmp.Or(v, msgUnknownValue)
	}
	return labels
}

// chooseTorrentFilter applies the combined filter steps; ok is false for
// other actions.
func (e *Engine) chooseTorrentFilter(_ context.Context, sess session, p press) (Reply, bool) {
	if !torrentFilterAction(p.action) {
		return Reply{}, false
	}
	if sess.focus == nil || len(sess.torrents) == 0 {
		return Reply{Notice: msgInvalidChoice}, true
	}
	if sess.streaming && sess.hunt != nil {
		sess = syncedHunt(sess, sess.hunt.read())
	}
	if p.action == actionTorrentFilterField {
		sess.filterOffers++
	}
	f, ok := nextFilter(sess, p)
	if !ok {
		return Reply{Notice: msgInvalidChoice}, true
	}
	sess.releaseFilter, sess.searchView = f, torrentFilterView
	return e.listPages(sess, filterPanel(sess)), true
}

// nextFilter is the filter once p applies; ok is false for a choice it
// does not offer.
func nextFilter(sess session, p press) (releaseFilter, bool) {
	f := sess.releaseFilter
	switch p.action {
	case actionTorrentFilterField:
		if p.arg < 0 || p.arg >= len(releaseFields) {
			return f, false
		}
		f.open, f.offered, f.offer = p.arg+1, offeredValues(sess, p.arg), sess.filterOffers
	case actionTorrentFilterTick:
		return f.tick(p.arg)
	case actionTorrentFilterClear:
		f = f.cleared(p.arg)
	default:
		f.open, f.offered = 0, nil
	}
	return f, true
}

// tick toggles the value arg picks among the open field's offered values;
// ok is false for a button from another field or an earlier opening.
func (f releaseFilter) tick(arg int) (releaseFilter, bool) {
	index := arg % filterOfferBase
	if f.open == 0 || arg < 0 || arg/filterOfferBase != f.offer || index >= len(f.offered) {
		return f, false
	}
	return f.with(f.open-1, toggledValue(f.ticked(f.open-1), f.offered[index])), true
}

func torrentFilterAction(action string) bool {
	switch action {
	case actionTorrentFilter, actionTorrentFilterField, actionTorrentFilterTick, actionTorrentFilterClear:
		return true
	}
	return false
}

// with is f with field narrowed to values.
func (f releaseFilter) with(field int, values []string) releaseFilter {
	picked := make([][]string, len(releaseFields))
	copy(picked, f.picked)
	picked[field] = values
	f.picked = picked
	return f
}

// cleared is f without the ticks of field arg-1, or of every field for 0.
func (f releaseFilter) cleared(arg int) releaseFilter {
	if arg <= 0 || arg > len(releaseFields) {
		f.picked = nil
		return f
	}
	return f.with(arg-1, nil)
}

func toggledValue(values []string, v string) []string {
	if i := slices.Index(values, v); i >= 0 {
		return slices.Delete(slices.Clone(values), i, i+1)
	}
	return append(slices.Clone(values), v)
}

// offeredValues are field's values among all the releases, unknown last;
// fixed bands keep their order, others go most releases first.
func offeredValues(sess session, field int) []string {
	counts := map[string]int{}
	for _, t := range sess.activeResources() {
		counts[releaseFields[field].value(t)]++
	}
	var values []string
	if bands := releaseFields[field].bands; bands != nil {
		values = slices.DeleteFunc(slices.Clone(bands), func(v string) bool { return counts[v] == 0 })
	} else {
		for v := range counts {
			if v != "" {
				values = append(values, v)
			}
		}
		slices.SortFunc(values, func(a, b string) int { return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b)) })
	}
	if counts[""] > 0 {
		values = append(values, "")
	}
	return values
}

// facetCount is how many releases shown under the other conditions have
// value for field, so ticking it says what it would add.
func facetCount(sess session, field int, value string) int {
	n := 0
	for _, i := range sess.activeTorrents() {
		t := sess.torrents[i]
		if sess.listedBySite(t) && sess.releaseFilter.passes(t, field) && releaseFields[field].value(t) == value {
			n++
		}
	}
	return n
}

// filterPanel shows each field's ticks, or the open field's values to
// tick; 返回 shows the list as now filtered.
func filterPanel(sess session) listView {
	f := sess.releaseFilter
	heading := "🔍 " + msgReleaseFilter
	note := fmt.Sprintf("符合 %d 个，共 %d 个。", len(sess.shownTorrents()), len(sess.activeTorrents()))
	view := listView{
		menu:   [][]Button{},
		footer: []Button{backTo(sess.id, actionTorrentSort, sess.torrentSort), cancelButton(sess.id)},
	}
	if f.open == 0 {
		view.note = note + msgFilterFields
		for i, field := range releaseFields {
			view.entries = append(view.entries, listEntry{text: Lines(Line(Strong(field.label+"："), Plain(tickedText(f.ticked(i)))))})
		}
	} else {
		field := f.open - 1
		heading += " · " + releaseFields[field].label
		view.note = note + msgFilterTicks
		view.entries = valueEntries(sess, field)
		clear := Button{Label: markShown(msgAnyValue, len(f.ticked(field)) == 0), Data: data(sess.id, actionTorrentFilterClear, field+1)}
		view.menu = append(view.menu, []Button{clear})
	}
	view.heading = Heading(Plain(heading))
	view.menu = append(view.menu, grid(fieldButtons(sess), fieldColumns)...)
	if f.active() {
		view.menu = append(view.menu, []Button{{Label: msgClearFilters, Data: data(sess.id, actionTorrentFilterClear, 0)}})
	}
	return view
}

func tickedText(values []string) string {
	if len(values) == 0 {
		return msgAnyValue
	}
	return strings.Join(valueLabels(values), "、")
}

// valueEntries number the open field's values with how many releases each
// would show, ticked ones tagged.
func valueEntries(sess session, field int) []listEntry {
	ticked := sess.releaseFilter.ticked(field)
	var entries []listEntry
	for i, v := range sess.releaseFilter.offered {
		item := entry(i+1, Strong(cmp.Or(v, msgUnknownValue)), fmt.Sprintf("%d 个", facetCount(sess, field, v)))
		label := fmt.Sprint(i + 1)
		if slices.Contains(ticked, v) {
			item.Tag, label = msgTickedTag, "✓ "+label
		}
		entries = append(entries, listEntry{
			text: Lines(item), buttons: []Button{{Label: label, Data: data(sess.id, actionTorrentFilterTick, sess.releaseFilter.offer*filterOfferBase+i)}},
		})
	}
	return entries
}

func fieldButtons(sess session) []Button {
	buttons := make([]Button, len(releaseFields))
	for i, field := range releaseFields {
		buttons[i] = Button{Label: markShown(field.label, sess.releaseFilter.open == i+1), Data: data(sess.id, actionTorrentFilterField, i)}
	}
	return buttons
}
