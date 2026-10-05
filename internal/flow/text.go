package flow

import (
	"fmt"
	"strings"
)

// maxCast bounds how many actors the card lists.
const maxCast = 3

// card is the picked media with everything known about it.
type card struct {
	Media   Media
	Details Details
}

// reply shows the card with its poster above message and buttons.
func (c card) reply(message string, buttons [][]Button) Reply {
	return Reply{Text: c.text() + "\n\n" + message, Image: c.Media.PosterURL, Buttons: buttons}
}

// text renders e.g.
//
//	沙丘 (2021) · 电影 · ⭐ 7.8
//	Dune
//	科幻 / 冒险 · 156 分钟
//	主演：提莫西·查拉梅、丽贝卡·弗格森
//	<overview>
func (c card) text() string {
	m, d := c.Media, c.Details
	lines := []string{joinNonEmpty(" · ", titleYear(m), m.Kind.String(), rating(m.Rating))}
	if m.OriginalTitle != "" && m.OriginalTitle != m.Title {
		lines = append(lines, m.OriginalTitle)
	}
	if facts := joinNonEmpty(" · ", strings.Join(d.Genres, " / "), length(d)); facts != "" {
		lines = append(lines, facts)
	}
	if len(d.Cast) > 0 {
		lines = append(lines, "主演："+strings.Join(d.Cast[:min(len(d.Cast), maxCast)], "、"))
	}
	if m.Overview != "" {
		lines = append(lines, truncate(m.Overview, overviewRunes))
	}
	return strings.Join(lines, "\n")
}

// resultLine is one numbered search result, e.g. "1. 沙丘 (2021) · 电影 · ⭐ 7.8 · Dune".
func resultLine(n int, m Media) string {
	original := ""
	if m.OriginalTitle != m.Title {
		original = m.OriginalTitle
	}
	return fmt.Sprintf("%d. %s", n, joinNonEmpty(" · ", titleYear(m), m.Kind.String(), rating(m.Rating), original))
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
