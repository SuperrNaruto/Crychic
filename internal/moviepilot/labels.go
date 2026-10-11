package moviepilot

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	seasonPattern  = regexp.MustCompile(`^S(\d+)`)
	episodePattern = regexp.MustCompile(`^E(\d+)(?:-E(\d+))?$`)
)

// parseSeason reads "S03" as 3, nil when there is no season.
func parseSeason(label string) *int {
	m := seasonPattern.FindStringSubmatch(label)
	if m == nil {
		return nil
	}
	season, _ := strconv.Atoi(m[1])
	return &season
}

// parseEpisodes reads MoviePilot's "E01-E03、E05" labels without filling gaps.
func parseEpisodes(label string) []int {
	var episodes []int
	for _, part := range strings.Split(label, "、") {
		m := episodePattern.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			continue
		}
		first, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		last := first
		if m[2] != "" {
			last, err = strconv.Atoi(m[2])
		}
		if err == nil {
			episodes = append(episodes, episodeRange(first, last)...)
		}
	}
	slices.Sort(episodes)
	return slices.Compact(episodes)
}

// episodeRange expands an inclusive range; invalid or unknown bounds stay empty.
func episodeRange(first, last int) []int {
	if first <= 0 || last < first {
		return nil
	}
	episodes := make([]int, last-first+1)
	for i := range episodes {
		episodes[i] = first + i
	}
	return episodes
}
