package flow

import (
	"fmt"
	"slices"
)

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
		note:      fmt.Sprintf("共 %d 个，%s，%s", len(shown), torrentSorts[sess.torrentSort].note, listHint(sess)),
		pageItems: torrentPageItems,
		footer:    []Button{cancelButton(sess.id)},
	}
	if sess.ticking {
		label := fmt.Sprintf("下载所选 %d 个", len(sess.ticked))
		view.menu = append(view.menu, []Button{{Label: label, Data: data(sess.id, actionTorrentBatch, 0)}})
	}
	view.menu = append(view.menu, sortRow(sess))
	if row := onwardRow(sess); len(row) > 0 {
		view.menu = append(view.menu, row)
	}
	for n, i := range shown {
		view.entries = append(view.entries, releaseEntry(sess, n+1, i))
	}
	return view
}

// listHint says what a number button does on the release list.
func listHint(sess session) string {
	if sess.ticking {
		return fmt.Sprintf("已选 %d 个，点编号勾选，再点一次取消～", len(sess.ticked))
	}
	return "点编号看详情～"
}

// onwardRow offers the site filter, when there is more than one site, and
// ticking several releases, unless they are being ticked.
func onwardRow(sess session) []Button {
	var row []Button
	if sess.torrentSite != "" || len(torrentSites(sess.torrents)) > 1 {
		row = append(row, Button{Label: msgSiteFilter, Data: data(sess.id, actionTorrentSites, 0)})
	}
	if !sess.ticking && len(sess.torrents) > 1 {
		row = append(row, Button{Label: msgTickReleases, Data: data(sess.id, actionTorrentMulti, 0)})
	}
	return row
}

// releaseEntry is release index listed as number n: its number button
// opens it, or ticks it while releases are ticked.
func releaseEntry(sess session, n, index int) listEntry {
	item := torrentEntry(n, sess.torrents[index])
	button := Button{Label: fmt.Sprint(n), Data: data(sess.id, actionTorrentPick, index)}
	if sess.ticking {
		button.Data = data(sess.id, actionTorrentTick, index)
		if slices.Contains(sess.ticked, index) {
			item.Tag, button.Label = msgTickedTag, "✓ "+button.Label
		}
	}
	return listEntry{text: Lines(item), buttons: []Button{button}}
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
