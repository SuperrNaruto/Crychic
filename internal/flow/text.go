package flow

import "fmt"

func titleYear(m Media) string {
	if m.Year == "" {
		return m.Title
	}
	return fmt.Sprintf("%s (%s)", m.Title, m.Year)
}

// targetName is how a subscription is referred to, e.g. 《沙丘》第 2 季.
func targetName(t Target) string {
	name := fmt.Sprintf("《%s》", t.Media.Title)
	if t.Season == nil {
		return name
	}
	return fmt.Sprintf("%s第 %d 季", name, *t.Season)
}

// describe is the media card shown above season and confirm choices.
func describe(m Media) string {
	head := fmt.Sprintf("%s · %s", titleYear(m), m.Kind)
	if m.Overview == "" {
		return head
	}
	return head + "\n" + truncate(m.Overview, overviewRunes)
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
