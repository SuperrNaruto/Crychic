package flow

import (
	"cmp"
	"context"
	"fmt"
	"slices"
)

const (
	actionTorrentSort  = "to" // arg: index into torrentSorts; shows the releases in that order
	actionTorrentSites = "tf" // lists the releases' sites to show one of them
	actionTorrentSite  = "tw" // arg: 0 for every site, else 1 + index into the sites

	siteColumns = 3

	msgAllSites   = "全部站点"
	msgPickSite   = "只看哪个站点的资源呀？"
	msgSiteFilter = "筛选站点"
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
	{label: "站点", note: "按站点名排列", less: func(a, b Torrent) int { return cmp.Compare(a.Site, b.Site) }},
}

// newerFirst orders by publish time, newest first; MoviePilot words it
// "2006-01-02 15:04:05", which sorts as text. Unknown times go last.
func newerFirst(a, b Torrent) int {
	switch {
	case a.Published == b.Published:
		return 0
	case a.Published == "":
		return 1
	case b.Published == "":
		return -1
	}
	return cmp.Compare(b.Published, a.Published)
}

// chooseTorrentView applies the sort and site filter of the release list;
// ok is false for other actions.
func (e *Engine) chooseTorrentView(_ context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionTorrentSort:
		if p.arg < 0 || p.arg >= len(torrentSorts) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		sess.torrentSort = p.arg
		return e.releases(sess), true
	case actionTorrentSites:
		return sitePicker(sess), true
	case actionTorrentSite:
		sites := torrentSites(sess.torrents)
		if p.arg < 0 || p.arg > len(sites) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		sess.torrentSite = ""
		if p.arg > 0 {
			sess.torrentSite = sites[p.arg-1].name
		}
		return e.releases(sess), true
	}
	return Reply{}, false
}

// releases lists the session's releases as sorted and filtered now.
func (e *Engine) releases(sess session) Reply {
	if sess.focus == nil || len(sess.torrents) == 0 {
		return Reply{Notice: msgInvalidChoice}
	}
	e.store.put(sess)
	return e.listPages(sess, torrentList(sess, *sess.focus))
}

// shownTorrents are the indices into the session's releases to list: those
// of the chosen site, in the chosen order.
func (sess session) shownTorrents() []int {
	var shown []int
	for i, t := range sess.torrents {
		if sess.torrentSite == "" || t.Site == sess.torrentSite {
			shown = append(shown, i)
		}
	}
	if less := torrentSorts[sess.torrentSort].less; less != nil {
		slices.SortStableFunc(shown, func(a, b int) int { return less(sess.torrents[a], sess.torrents[b]) })
	}
	return shown
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
		i := slices.IndexFunc(sites, func(s siteCount) bool { return s.name == t.Site })
		if i < 0 {
			sites = append(sites, siteCount{name: t.Site})
			i = len(sites) - 1
		}
		sites[i].count++
	}
	slices.SortStableFunc(sites, func(a, b siteCount) int { return cmp.Compare(b.count, a.count) })
	return sites
}

// sitePicker offers each site of the releases, and every site; 返回 shows
// the list as it was.
func sitePicker(sess session) Reply {
	sites := torrentSites(sess.torrents)
	text := Lines(Heading(Plain("🔍 "+msgSiteFilter)), Line(Strong(msgPickSite)))
	buttons := []Button{{Label: markShown(msgAllSites, sess.torrentSite == ""), Data: data(sess.id, actionTorrentSite, 0)}}
	for i, s := range sites {
		text = append(text, Line(Plain(fmt.Sprintf("%s · %d 个", s.name, s.count))))
		buttons = append(buttons, Button{Label: markShown(s.name, s.name == sess.torrentSite), Data: data(sess.id, actionTorrentSite, i+1)})
	}
	rows := grid(buttons, siteColumns)
	rows = append(rows, []Button{backTo(sess.id, actionTorrentSort, sess.torrentSort), cancelButton(sess.id)})
	return Reply{Text: text, Buttons: rows}
}
