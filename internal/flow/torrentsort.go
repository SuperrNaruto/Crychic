package flow

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
)

const (
	actionTorrentView  = "tv" // opens 排序与筛选
	actionTorrentSort  = "tc" // arg: index into torrentSorts; picks the order on 排序与筛选
	actionTorrentList  = "ta" // shows the release list as sorted and filtered now
	actionTorrentSites = "tf" // lists the releases' sites to show one of them
	actionTorrentSite  = "tw" // arg: 0 for every site, else 1 + index into the sites

	siteColumns = 3

	msgSortFilter = "排序与筛选"
	msgAllSites   = "全部站点"
	msgPickSite   = "只看哪个站点的资源呀？"
	msgSiteFilter = "筛选站点"
	// msgUnknownSite names a release's site when the backend gave none, so
	// it can be filtered and never makes an empty button.
	msgUnknownSite = "未知站点"
)

// torrentSort is one order to show the releases in, as MoviePilot's
// WebUI offers them.
type torrentSort struct {
	label string
	note  string
	less  func(a, b Torrent) int // nil keeps MoviePilot's order
}

var torrentSorts = []torrentSort{
	{label: "默认", note: "按 MoviePilot 的优先级排好啦"},
	{label: "做种", note: "做种人数从多到少", less: func(a, b Torrent) int { return cmp.Compare(b.Seeders, a.Seeders) }},
	{label: "时间", note: "发布时间从新到旧", less: newerFirst},
	{label: "大小", note: "体积从大到小", less: func(a, b Torrent) int { return cmp.Compare(b.Size, a.Size) }},
	{label: "站点", note: "按站点名排列", less: func(a, b Torrent) int { return cmp.Compare(siteOf(a), siteOf(b)) }},
}

// siteOf is the site a release is listed and filtered under.
func siteOf(t Torrent) string {
	return cmp.Or(t.Site, msgUnknownSite)
}

// newerFirst orders by publish time, newest first; MoviePilot words it
// "2006-01-02 15:04:05", which sorts as text. Descending string order also
// puts empty (unknown) times last.
func newerFirst(a, b Torrent) int {
	return cmp.Compare(b.Published, a.Published)
}

// chooseTorrentView applies the sort and filters of the release list on
// 排序与筛选, which every step but 返回 redraws in place; ok is false for
// other actions.
func (e *Engine) chooseTorrentView(_ context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionTorrentView:
		return e.sortFilterPanel(sess), true
	case actionTorrentList:
		return e.releases(sess), true
	case actionTorrentSort:
		if p.arg < 0 || p.arg >= len(torrentSorts) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		sess.torrentSort = p.arg
		return e.sortFilterPanel(sess), true
	case actionTorrentSites:
		sess.searchView = torrentFilterView
		e.store.put(sess)
		return sitePicker(sess), true
	case actionTorrentSite:
		return e.pickSite(sess, p.arg), true
	case actionTorrentMissing:
		sess.missingOnly = p.arg == 1
		return e.sortFilterPanel(sess), true
	}
	return Reply{}, false
}

// pickSite shows only the releases of site arg (0 for every site) and goes
// back to 排序与筛选.
func (e *Engine) pickSite(sess session, arg int) Reply {
	sites := torrentSites(sess.activeResources())
	if arg < 0 || arg > len(sites) {
		return Reply{Notice: msgInvalidChoice}
	}
	sess.torrentSite = ""
	if arg > 0 {
		sess.torrentSite = sites[arg-1].name
	}
	return e.sortFilterPanel(sess)
}

// sortFilterPanel shows the current order and filters of the releases with
// the controls to change them; 返回 shows the list as it is by then.
func (e *Engine) sortFilterPanel(sess session) Reply {
	if sess.focus == nil || len(sess.torrents) == 0 {
		return Reply{Notice: msgInvalidChoice}
	}
	if sess.streaming && sess.hunt != nil {
		sess = syncedHunt(sess, sess.hunt.read())
	}
	sess.searchView = torrentFilterView
	return e.listPages(sess, sortFilterView(sess))
}

func sortFilterView(sess session) listView {
	order := torrentSorts[sess.torrentSort]
	view := listView{
		heading: Heading(Plain("🔍 " + msgSortFilter)),
		note:    releaseCount(sess, len(sess.shownTorrents())) + "，调好后点「返回」看结果～",
		entries: []listEntry{
			{text: Lines(Line(Strong("排序："), Plain(order.label+"，"+order.note)))},
			{text: Lines(Line(Strong("筛选："), Plain(filterText(sess))))},
		},
		menu:   [][]Button{sortRow(sess)},
		footer: []Button{backTo(sess.id, actionTorrentList, 0), cancelButton(sess.id)},
	}
	if row := missingSwitch(sess); len(row) > 0 {
		view.menu = append(view.menu, row)
	}
	if row := filterRow(sess); len(row) > 0 {
		view.menu = append(view.menu, row)
	}
	return view
}

// filterText names the active conditions, 不限 when there are none.
func filterText(sess session) string {
	var parts []string
	if sess.torrentSite != "" {
		parts = append(parts, "站点 "+sess.torrentSite)
	}
	if sess.missingOnly {
		parts = append(parts, msgMissingOnly)
	}
	if sess.releaseFilter.active() {
		parts = append(parts, sess.releaseFilter.summary())
	}
	if len(parts) == 0 {
		return msgAnyValue
	}
	return strings.Join(parts, " · ")
}

// filterRow offers the site filter, when there is more than one site, and
// the combined filter.
func filterRow(sess session) []Button {
	var row []Button
	if sess.torrentSite != "" || len(torrentSites(sess.activeResources())) > 1 {
		row = append(row, Button{Label: msgSiteFilter, Data: data(sess.id, actionTorrentSites, 0)})
	}
	if sess.releaseFilter.active() || len(sess.activeTorrents()) > 1 {
		row = append(row, Button{Label: msgReleaseFilter, Data: data(sess.id, actionTorrentFilter, 0)})
	}
	return row
}

// releases lists the session's releases as sorted and filtered now.
func (e *Engine) releases(sess session) Reply {
	if sess.focus == nil || len(sess.torrents) == 0 {
		return Reply{Notice: msgInvalidChoice}
	}
	if sess.streaming {
		sess = syncedHunt(sess, sess.hunt.read())
		sess.pageIndex = 0
		return e.streamReleases(sess)
	}
	return e.listPages(sess, torrentList(sess, *sess.focus))
}

// shownTorrents are the indices into the session's releases to list: those
// of the chosen site, bringing missing episodes if asked and passing the
// combined filter, in the chosen order.
func (sess session) shownTorrents() []int {
	var shown []int
	for _, i := range sess.activeTorrents() {
		t := sess.torrents[i]
		if sess.listedBySite(t) && sess.releaseFilter.passes(t, -1) {
			shown = append(shown, i)
		}
	}
	if less := torrentSorts[sess.torrentSort].less; less != nil {
		slices.SortStableFunc(shown, func(a, b int) int { return less(sess.torrents[a], sess.torrents[b]) })
	}
	return shown
}

// listedBySite reports whether t is of the chosen site and, if asked,
// brings missing episodes; the combined filter applies on top.
func (sess session) listedBySite(t Torrent) bool {
	return (sess.torrentSite == "" || siteOf(t) == sess.torrentSite) && (!sess.missingOnly || sess.fillsMissing(t))
}

func (sess session) activeTorrents() []int {
	if sess.streaming {
		return slices.Clone(sess.torrentOrder)
	}
	indices := make([]int, len(sess.torrents))
	for i := range indices {
		indices[i] = i
	}
	return indices
}

func (sess session) activeResources() []Torrent {
	var out []Torrent
	for _, i := range sess.activeTorrents() {
		out = append(out, sess.torrents[i])
	}
	return out
}

// sortRow switches the release order, the shown one marked.
func sortRow(sess session) []Button {
	row := make([]Button, 0, len(torrentSorts))
	for i, s := range torrentSorts {
		row = append(row, Button{Label: markShown(s.label, i == sess.torrentSort), Data: data(sess.id, actionTorrentSort, i)})
	}
	return row
}

// siteCount is a site and how many of the releases it has.
type siteCount struct {
	name  string
	count int
}

// torrentSites are the releases' sites, most releases first.
func torrentSites(torrents []Torrent) []siteCount {
	var sites []siteCount
	for _, t := range torrents {
		i := slices.IndexFunc(sites, func(s siteCount) bool { return s.name == siteOf(t) })
		if i < 0 {
			sites = append(sites, siteCount{name: siteOf(t)})
			i = len(sites) - 1
		}
		sites[i].count++
	}
	slices.SortStableFunc(sites, func(a, b siteCount) int { return cmp.Compare(b.count, a.count) })
	return sites
}

// sitePicker offers each site of the releases, and every site; 返回 goes
// back to 排序与筛选.
func sitePicker(sess session) Reply {
	sites := torrentSites(sess.activeResources())
	text := Lines(Heading(Plain("🔍 "+msgSiteFilter)), Line(Strong(msgPickSite)))
	buttons := []Button{{Label: markShown(msgAllSites, sess.torrentSite == ""), Data: data(sess.id, actionTorrentSite, 0)}}
	for i, s := range sites {
		text = append(text, Line(Plain(fmt.Sprintf("%s · %d 个", s.name, s.count))))
		buttons = append(buttons, Button{Label: markShown(s.name, s.name == sess.torrentSite), Data: data(sess.id, actionTorrentSite, i+1)})
	}
	rows := grid(buttons, siteColumns)
	rows = append(rows, []Button{backTo(sess.id, actionTorrentView, 0), cancelButton(sess.id)})
	return Reply{Text: text, Buttons: rows}
}
