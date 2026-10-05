package flow

import (
	"fmt"
	"strings"
)

const (
	// maxCast bounds how many actors the card lists.
	maxCast = 3
	// maxOverviewRunes keeps the collapsed synopsis well inside chat limits.
	maxOverviewRunes = 1000
)

// card is the picked media with everything known about it.
type card struct {
	Media     Media
	Details   Details
	Downloads []Download // the chosen target's unfinished downloads
}

// reply shows the card, poster above, then message and buttons.
func (c card) reply(message Block, buttons [][]Button) Reply {
	return c.replyLines(Lines(message), buttons)
}

// replyLines is reply with a message of several lines.
func (c card) replyLines(message Text, buttons [][]Button) Reply {
	text := append(append(c.text(), Line()), message...)
	return Reply{Text: text, Image: c.Media.PosterURL, Buttons: buttons}
}

// text renders e.g.
//
//	🎬 **沙丘** (2021) · 电影 · ⭐ 7.8      title linked to its page
//	_Dune_
//	科幻 / 冒险 · 156 分钟
//	主演：提莫西·查拉梅、丽贝卡·弗格森
//	> synopsis, collapsed
func (c card) text() Text {
	m, d := c.Media, c.Details
	icon := "🎬 "
	if m.Kind == TV {
		icon = "📺 "
	}
	head := []Span{Plain(icon), Linked(Strong(m.Title), m.Link)}
	head = append(head, Plain(" "+joinNonEmpty(" · ", year(m), m.Kind.String(), rating(m.Rating))))
	text := Lines(Line(head...))
	if m.OriginalTitle != "" && m.OriginalTitle != m.Title {
		text = append(text, Line(Emphasis(m.OriginalTitle)))
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

// resultLine is one numbered search result, e.g.
// "1. **沙丘** (2021) · 电影 · ⭐ 7.8 · _Dune_".
func resultLine(n int, m Media) Block {
	spans := []Span{Plain(fmt.Sprintf("%d. ", n)), Strong(m.Title)}
	spans = append(spans, Plain(" "+joinNonEmpty(" · ", year(m), m.Kind.String(), rating(m.Rating))))
	if m.OriginalTitle != "" && m.OriginalTitle != m.Title {
		spans = append(spans, Plain(" · "), Emphasis(m.OriginalTitle))
	}
	return Line(spans...)
}

// year is "(2021)", or empty when unknown.
func year(m Media) string {
	if m.Year == "" {
		return ""
	}
	return "(" + m.Year + ")"
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

// seasonLines asks question above a line per season.
func (sess session) seasonLines(question string, seasons []Season) Text {
	text := Lines(Line(Strong(question)))
	for _, s := range seasons {
		name, facts, _ := strings.Cut(sess.seasonLabel(s), " · ")
		line := Line(Strong(name))
		if facts != "" {
			line = Line(Strong(name), Plain(" · "+facts))
		}
		text = append(text, line)
	}
	return text
}

// seasonLabel describes a season, e.g. "第 2 季 · 13 集 · 已有 5 集".
func (sess session) seasonLabel(s Season) string {
	label := seasonName(s.Number)
	if s.EpisodeCount > 0 {
		label = fmt.Sprintf("%s · %d 集", label, s.EpisodeCount)
	}
	held := len(sess.library.Episodes[s.Number])
	switch {
	case sess.wholeSeasonHeld(s):
		label += " · 已入库"
	case held > 0:
		label += fmt.Sprintf(" · 已有 %d 集", held)
	}
	return label
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
