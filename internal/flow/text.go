package flow

import (
	"fmt"
	"slices"
	"strings"
)

const (
	// maxCast bounds how many actors the card lists.
	maxCast = 3
	// maxOverviewRunes keeps the collapsed synopsis well inside chat limits.
	maxOverviewRunes = 1000
)

// seasonHead names the season picker's table columns.
var seasonHead = []string{"季", "集数", "入库"}

// card is the picked media with everything known about it.
type card struct {
	Media     Media
	Details   Details
	Downloads []Download // the chosen target's unfinished downloads
}

// reply shows the card, poster above, then a divider, message and buttons.
func (c card) reply(message Block, buttons [][]Button) Reply {
	return c.replyLines(Lines(message), buttons)
}

// replyLines is reply with a message of several lines.
func (c card) replyLines(message Text, buttons [][]Button) Reply {
	text := append(append(c.text(), Divider()), message...)
	return Reply{Text: text, Image: c.Media.PosterURL, Buttons: buttons}
}

// text renders e.g.
//
//	# 🎬 沙丘                       heading, title linked to its page
//	_Dune_
//	2021 · 电影 · ⭐ 7.8
//	科幻 / 冒险 · 156 分钟
//	主演：提莫西·查拉梅、丽贝卡·弗格森
//	> synopsis, collapsed
func (c card) text() Text {
	m, d := c.Media, c.Details
	icon := "🎬 "
	if m.Kind == TV {
		icon = "📺 "
	}
	text := Lines(Heading(Plain(icon), Linked(Plain(m.Title), m.Link)))
	if m.OriginalTitle != "" && m.OriginalTitle != m.Title {
		text = append(text, Line(Emphasis(m.OriginalTitle)))
	}
	if facts := joinNonEmpty(" · ", m.Year, m.Kind.String(), rating(m.Rating)); facts != "" {
		text = append(text, Line(Plain(facts)))
	}
	if facts := joinNonEmpty(" · ", strings.Join(d.Genres, " / "), length(d)); facts != "" {
		text = append(text, Line(Plain(facts)))
	}
	if len(d.Cast) > 0 {
		text = append(text, Line(Plain("主演："+strings.Join(d.Cast[:min(len(d.Cast), maxCast)], "、"))))
	}
	if m.Overview != "" {
		text = append(text, Quote(truncate(m.Overview, maxOverviewRunes)))
	}
	for _, dl := range c.Downloads {
		text = append(text, downloadLine(dl))
	}
	return text
}

// downloadLine is e.g. "⬇️ 正在下载 E10–E12 · 37% · 剩余 1时5分3秒".
func downloadLine(d Download) Block {
	head := "⬇️ 正在下载"
	if d.Paused {
		head = "⏸️ 下载已暂停"
	}
	if len(d.Episodes) > 0 {
		head += " " + EpisodeRanges(d.Episodes)
	}
	left := ""
	if d.Left != "" {
		left = "剩余 " + d.Left
	}
	return Line(Plain(joinNonEmpty(" · ", head, fmt.Sprintf("%.0f%%", d.Progress), left)))
}

// has reports whether download d is for this target: same media and, for a
// season, the same season when the download names one.
func (t Target) has(d Download) bool {
	if d.Source != t.Media.Source || d.MediaID != t.Media.ID {
		return false
	}
	return t.Season == nil || d.Season == nil || *d.Season == *t.Season
}

// entry is a numbered list entry: its name, then the facts about it on a
// secondary line of their own, so a fact never wraps under the numbers.
func entry(n int, name Span, facts ...string) Block {
	return Block{Kind: Item, Number: n, Spans: []Span{name}, Facts: joinNonEmpty(" · ", facts...)}
}

// resultEntry is one numbered search result: **沙丘**, then
// _Dune · 2021 · 电影 · ⭐ 7.8_ below.
func resultEntry(n int, m Media) Block {
	original := ""
	if m.OriginalTitle != m.Title {
		original = m.OriginalTitle
	}
	return entry(n, Strong(m.Title), original, m.Year, m.Kind.String(), rating(m.Rating))
}

func length(d Details) string {
	if d.Runtime > 0 {
		return fmt.Sprintf("%d 分钟", d.Runtime)
	}
	if d.Seasons > 0 {
		return fmt.Sprintf("共 %d 季 %d 集", d.Seasons, d.Episodes)
	}
	return ""
}

func rating(r float64) string {
	if r <= 0 {
		return ""
	}
	return fmt.Sprintf("⭐ %.1f", r)
}

func joinNonEmpty(sep string, parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

// targetName is how a subscription is referred to, e.g. 《沙丘》,
// 《绝命毒师》第 2 季 or 《名侦探柯南》第 1 季（从第 500 集开始）.
func targetName(t Target) string {
	name := fmt.Sprintf("《%s》", t.Media.Title)
	if t.Season == nil {
		return name
	}
	name = fmt.Sprintf("%s第 %d 季", name, *t.Season)
	if t.StartEpisode > 0 {
		name += fmt.Sprintf("（从第 %d 集开始）", t.StartEpisode)
	}
	return name
}

// seasonName is "第 2 季", or 特别篇 for season 0.
func seasonName(number int) string {
	if number == 0 {
		return "特别篇"
	}
	return fmt.Sprintf("第 %d 季", number)
}

// seasonTable asks question above a table of the seasons: name, episode
// count and what the library holds.
func (sess session) seasonTable(question string, seasons []Season) Text {
	rows := make([][]Span, 0, len(seasons))
	for _, s := range seasons {
		rows = append(rows, []Span{Strong(seasonName(s.Number)), Plain(episodeCount(s)), Plain(sess.seasonHeld(s))})
	}
	return Lines(Line(Strong(question)), Table(seasonHead, rows...))
}

// seasonChecklist asks question above a checklist of the seasons, those in
// chosen ticked, e.g. "☑ **第 2 季** · 13 集 · 已有 5 集".
func (sess session) seasonChecklist(question string, seasons []Season, chosen []int) Text {
	text := Lines(Line(Strong(question)))
	for _, s := range seasons {
		spans := []Span{Strong(seasonName(s.Number))}
		if facts := joinNonEmpty(" · ", episodeCount(s), sess.seasonHeld(s)); facts != "" {
			spans = append(spans, Plain(" · "+facts))
		}
		text = append(text, Ticked(slices.Contains(chosen, s.Number), spans...))
	}
	return text
}

// episodeCount is e.g. "13 集", empty while unknown.
func episodeCount(s Season) string {
	if s.EpisodeCount == 0 {
		return ""
	}
	return fmt.Sprintf("%d 集", s.EpisodeCount)
}

// seasonHeld is what the library holds of a season: 已入库, 已有 5 集 or
// nothing.
func (sess session) seasonHeld(s Season) string {
	held := len(sess.library.Episodes[s.Number])
	switch {
	case sess.wholeSeasonHeld(s):
		return "已入库"
	case held > 0:
		return fmt.Sprintf("已有 %d 集", held)
	}
	return ""
}

// EpisodeRanges renders sorted episode numbers compactly: E01–E03、E05.
func EpisodeRanges(eps []int) string {
	var parts []string
	for i := 0; i < len(eps); {
		j := i
		for j+1 < len(eps) && eps[j+1] == eps[j]+1 {
			j++
		}
		part := fmt.Sprintf("E%02d", eps[i])
		if j > i {
			part += fmt.Sprintf("–E%02d", eps[j])
		}
		parts = append(parts, part)
		i = j + 1
	}
	return strings.Join(parts, "、")
}

func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}
