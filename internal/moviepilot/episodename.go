package moviepilot

import (
	"regexp"
	"slices"
	"strconv"
)

var (
	// tokenEpisode is a title token naming an episode: E09, EP09, S01E09, or
	// a bare number as in 「MyGO - 09」; S01 alone names a season.
	tokenEpisode = regexp.MustCompile(`(?i)^(?:s\d{1,3})?(?:e|ep)(\d{1,4})(?:v\d)?$|^(\d{1,4})(?:v\d)?$`)
	// cnEpisode is 第9集, 第09话 or 第1-13集 anywhere in a title.
	cnEpisode = regexp.MustCompile(`第\s*(\d{1,4})(?:\s*-\s*(\d{1,4}))?\s*[集话話]`)
	// technical are numbers a title carries that name no episode: audio
	// channels (DDP5.1, AAC2.0, TrueHD.7.1.4) and H.264 / H.265.
	technical = regexp.MustCompile(`(?i)h\.26[45]|(^|\D)[1-9]\.[0-2](?:\.[0-4])?(\D|$)`)
	// titleToken is a run of a title between separators.
	titleToken = regexp.MustCompile(`[^.\s_\[\]()【】\-+]+`)
)

// namesEpisodes reports whether a release title itself names one of
// episodes. MoviePilot reads a release's episodes from its description
// only when its title names none, which misreads a whole-season pack whose
// description says 修复第9集章节 as E09.
func namesEpisodes(title string, episodes []int) bool {
	var named []string
	title = technical.ReplaceAllString(title, "$1 $2")
	for _, token := range titleToken.FindAllString(title, -1) {
		if m := tokenEpisode.FindStringSubmatch(token); m != nil {
			named = append(named, m[1]+m[2])
		}
	}
	for _, m := range cnEpisode.FindAllStringSubmatch(title, -1) {
		named = append(named, m[1], m[2])
	}
	for _, s := range named {
		n, err := strconv.Atoi(s)
		if err == nil && slices.Contains(episodes, n) {
			return true
		}
	}
	return false
}
