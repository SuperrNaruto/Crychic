package flow

import "slices"

const (
	actionTorrentMissing = "tx" // arg: 1 shows only releases filling missing episodes, 0 every release

	msgMissingOnly = "只看缺集"
	msgAllHeld     = "已都有"
)

// missingEpisodes are the episodes of the focus season the library lacks,
// from the chosen start on. ok is false when the season, its length or the
// library is unknown, or the library holds none of them: every release then
// only brings missing episodes and there is nothing to tell apart.
func (sess session) missingEpisodes() (missing []int, ok bool) {
	if sess.focus == nil || sess.focus.Season == nil || sess.picked.LibraryUnknown {
		return nil, false
	}
	season := *sess.focus.Season
	count := sess.episodeCount(season)
	held := sess.library.Episodes[season]
	from := max(sess.focus.StartEpisode, 1)
	heldAny := false
	for ep := from; ep <= count; ep++ {
		if slices.Contains(held, ep) {
			heldAny = true
			continue
		}
		missing = append(missing, ep)
	}
	return missing, heldAny
}

// fills is which of missing a release brings: all of them for a release of
// the whole season, none for another season's.
func fills(t Torrent, season int, missing []int) []int {
	if t.Season != nil && *t.Season != season {
		return nil
	}
	if len(t.Episodes) == 0 {
		return missing
	}
	var filled []int
	for _, ep := range t.Episodes {
		if slices.Contains(missing, ep) {
			filled = append(filled, ep)
		}
	}
	return filled
}

// fillNote is what a release adds to the library: 已都有 when nothing,
// 补 E01–E12 when only part of what it carries, "" when all of it is new
// (its own episodes already say so) or nothing is known.
func (sess session) fillNote(t Torrent) string {
	missing, ok := sess.missingEpisodes()
	if !ok || t.EpisodesUnsure {
		return ""
	}
	filled := fills(t, *sess.focus.Season, missing)
	switch {
	case len(filled) == 0:
		return msgAllHeld
	case len(t.Episodes) > 0 && len(filled) == len(t.Episodes):
		return ""
	}
	return "补 " + EpisodeRanges(filled)
}

// fillsMissing reports whether a release brings any missing episode; true
// for every release while that is unknown, as it is for episodes its title
// does not name.
func (sess session) fillsMissing(t Torrent) bool {
	missing, ok := sess.missingEpisodes()
	return !ok || t.EpisodesUnsure || len(fills(t, *sess.focus.Season, missing)) > 0
}

// missingSwitch is the 只看缺集 switch, offered while some release brings
// nothing missing or the filter is on.
func missingSwitch(sess session) []Button {
	if !sess.missingOnly && !slices.ContainsFunc(sess.torrents, func(t Torrent) bool { return !sess.fillsMissing(t) }) {
		return nil
	}
	arg := 1
	if sess.missingOnly {
		arg = 0
	}
	return []Button{{Label: markShown(msgMissingOnly, sess.missingOnly), Data: data(sess.id, actionTorrentMissing, arg)}}
}
