package flow

import "slices"

// suggestTorrentPlan uses only the final snapshot's upstream priority and the
// selected site. Display sorting, paging and missing-only filtering cannot
// change the recommendation. This bounded greedy choice is not an optimum.
func suggestTorrentPlan(sess session, scope torrentPlanScope) *torrentPlan {
	scope.held = slices.Clone(scope.held)
	plan := &torrentPlan{scope: scope}
	if !scope.valid() || !validTorrentPlanTarget(sess.focus) {
		plan.problem = msgPlanInvalidScope
		return plan
	}
	plan.missing = scope.missing()
	candidates, excluded := torrentPlanCandidates(sess, scope)
	plan.excluded = excluded
	selected, limited := selectTorrentPlan(candidates, plan.missing)
	plan.limited = limited
	plan.summarize(selected)
	return plan
}

func validTorrentPlanTarget(target *Target) bool {
	if target == nil || target.Season == nil {
		return false
	}
	return target.Media.Kind == TV && target.Media.Source != "" && target.Media.ID != "" && *target.Season >= 0
}

func torrentPlanCandidates(sess session, scope torrentPlanScope) ([]torrentPlanCandidate, int) {
	var candidates []torrentPlanCandidate
	excluded := 0
	for _, index := range sess.activeTorrents() {
		release := sess.torrents[index]
		if sess.torrentSite != "" && siteOf(release) != sess.torrentSite {
			continue
		}
		candidate := torrentPlanCandidate{index: index, release: release}
		candidate.episodes = torrentPlanRange(release.Episodes, scope.count)
		if !trustedTorrentPlanRelease(release, *sess.focus) || len(candidate.episodes) == 0 {
			excluded++
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates, excluded
}

func trustedTorrentPlanRelease(release Torrent, target Target) bool {
	if release.Unverified || release.EpisodesUnsure || release.RangeUnsure {
		return false
	}
	return release.Season != nil && *release.Season == *target.Season && sameTorrentPlanMedia(release.Media, target.Media)
}

func sameTorrentPlanMedia(left, right Media) bool {
	return left.Source == right.Source && left.ID == right.ID && left.Kind == right.Kind
}

func torrentPlanRange(episodes []int, count int) []int {
	episodes = slices.Clone(episodes)
	slices.Sort(episodes)
	episodes = slices.Compact(episodes)
	if len(episodes) == 0 || episodes[0] < 1 || episodes[len(episodes)-1] > count {
		return nil
	}
	return episodes
}

func selectTorrentPlan(candidates []torrentPlanCandidate, missing []int) ([]torrentPlanCandidate, bool) {
	remaining := torrentPlanSet(missing)
	var selected []torrentPlanCandidate
	for len(selected) < maxTickedReleases {
		best := bestTorrentPlanCandidate(candidates, remaining)
		if best == noPlanCandidate {
			return selected, false
		}
		candidate := candidates[best]
		selected = append(selected, candidate)
		for _, ep := range candidate.episodes {
			remaining[ep] = false
		}
	}
	return selected, bestTorrentPlanCandidate(candidates, remaining) != noPlanCandidate
}

type torrentPlanRank struct {
	gained, repeated int
	hitAndRun        bool
	size             float64
}

func rankTorrentPlan(candidate torrentPlanCandidate, remaining torrentPlanEpisodeSet) torrentPlanRank {
	rank := torrentPlanRank{hitAndRun: candidate.release.HitAndRun, size: candidate.release.Size}
	for _, ep := range candidate.episodes {
		if remaining[ep] {
			rank.gained++
		}
	}
	rank.repeated = len(candidate.episodes) - rank.gained
	return rank
}

func (rank torrentPlanRank) better(other torrentPlanRank) bool {
	if rank.gained != other.gained {
		return rank.gained > other.gained
	}
	if rank.hitAndRun != other.hitAndRun {
		return !rank.hitAndRun
	}
	if rank.repeated != other.repeated {
		return rank.repeated < other.repeated
	}
	known, otherKnown := torrentPlanKnownSize(rank.size), torrentPlanKnownSize(other.size)
	if known != otherKnown {
		return known
	}
	return known && rank.size < other.size
}

func bestTorrentPlanCandidate(candidates []torrentPlanCandidate, remaining torrentPlanEpisodeSet) int {
	best, bestRank := noPlanCandidate, torrentPlanRank{}
	for index, candidate := range candidates {
		rank := rankTorrentPlan(candidate, remaining)
		if rank.gained > 0 && rank.better(bestRank) {
			best, bestRank = index, rank
		}
	}
	return best
}
