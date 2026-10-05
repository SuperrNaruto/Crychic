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
	Media   Media
	Details Details
}

// reply shows the card, poster above, then message and buttons.
func (c card) reply(message Block, buttons [][]Button) Reply {
	text := append(c.text(), Line(), message)
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
	return text
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

func titleYear(m Media) string {
	if m.Year == "" {
		return m.Title
	}
	return fmt.Sprintf("%s (%s)", m.Title, m.Year)
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

func seasonLabel(s Season) string {
	label := fmt.Sprintf("第 %d 季", s.Number)
	if s.Number == 0 {
		label = "特别篇"
	}
	if s.EpisodeCount > 0 {
		label = fmt.Sprintf("%s · %d 集", label, s.EpisodeCount)
	}
	return label
}

func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}
