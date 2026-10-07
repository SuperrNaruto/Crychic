package flow

import "fmt"

// torrentHead and qualityHead name the release card's two tables.
var (
	torrentHead = []string{"站点", "大小", "做种", "促销"}
	qualityHead = []string{"分辨率", "版本", "编码", "制作组"}
)

// torrentList numbers the releases found for target as sorted and
// filtered now; number buttons keep each release's own index.
func torrentList(sess session, target Target) listView {
	target.StartEpisode = 0
	heading := fmt.Sprintf("🔍 %s的资源", targetName(target))
	if sess.torrentSite != "" {
		heading += " · " + sess.torrentSite
	}
	shown := sess.shownTorrents()
	view := listView{
		heading:   Heading(Plain(heading)),
		note:      fmt.Sprintf("共 %d 个，%s，点编号看详情～", len(shown), torrentSorts[sess.torrentSort].note),
		pageItems: torrentPageItems,
		menu:      [][]Button{sortRow(sess), {{Label: msgSiteFilter, Data: data(sess.id, actionTorrentSites, 0)}}},
		footer:    []Button{cancelButton(sess.id)},
	}
	for n, i := range shown {
		view.entries = append(view.entries, listEntry{
			text:    Lines(torrentEntry(n+1, sess.torrents[i])),
			buttons: []Button{{Label: fmt.Sprint(n + 1), Data: data(sess.id, actionTorrentPick, i)}},
		})
	}
	return view
}

// torrentEntry is e.g. "1. **Dune.2021.2160p.UHD.BluRay…**" over
// "_站点A · 58.2 GiB · 做种 25 · 免费 · H&R_".
func torrentEntry(n int, t Torrent) Block {
	return entry(n, Strong(truncate(t.Title, listTitleRunes)),
		t.Site, size(t.Size), seeders(t), t.Promotion, hitAndRun(t), EpisodeRanges(t.Episodes))
}

// torrentCard is a release's title, its subtitle in small print, and its
// facts in two tables.
func torrentCard(t Torrent) Text {
	text := Lines(Line(Strong(t.Title)))
	if t.Description != "" {
		text = append(text, Small(Plain(t.Description)))
	}
	text = append(text,
		Table(torrentHead, []Span{Plain(t.Site), Plain(size(t.Size)), Plain(fmt.Sprint(t.Seeders)), Plain(t.Promotion)}),
		Table(qualityHead, []Span{Plain(t.Resolution), Plain(t.Edition), Plain(t.Video), Plain(t.Group)}),
	)
	if eps := EpisodeRanges(t.Episodes); eps != "" {
		text = append(text, Line(Plain("包含 "+eps)))
	}
	return text
}

// releaseName is what a download brings, e.g. 《迷途之子!!!!!》第 1 季 E01–E03.
func releaseName(target Target, t Torrent) string {
	target.StartEpisode = 0
	return joinNonEmpty(" ", targetName(target), EpisodeRanges(t.Episodes))
}

func seeders(t Torrent) string {
	return fmt.Sprintf("做种 %d", t.Seeders)
}

func hitAndRun(t Torrent) string {
	if t.HitAndRun {
		return "H&R"
	}
	return ""
}
