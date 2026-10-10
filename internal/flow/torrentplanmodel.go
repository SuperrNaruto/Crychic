package flow

import (
	"math"
	"slices"
)

const (
	maxPlanEpisodes     = 100
	noPlanCandidate     = -1
	msgPlanInvalidScope = "季集范围不确定，暂时不能生成缺集方案哦。"
)

// torrentPlanScope is a verified snapshot, with held sorted, unique and bounded
// to this season. Episodes before from may still be downloaded inside a pack.
type torrentPlanScope struct {
	count, from int
	held        []int
}

type torrentPlan struct {
	scope                                torrentPlanScope
	batch                                []int
	missing, covered, remaining, overlap []int
	repeated, extra                      []int
	excluded                             int
	limited                              bool
	totalSize                            float64
	unknownSizes, hitAndRun, unseeded    int
	problem                              string
}

func (scope torrentPlanScope) valid() bool {
	return scope.count > 0 && scope.count <= maxPlanEpisodes && scope.from > 0 && scope.from <= scope.count
}

func (scope torrentPlanScope) missing() []int {
	if !scope.valid() {
		return nil
	}
	var missing []int
	for ep := scope.from; ep <= scope.count; ep++ {
		if !slices.Contains(scope.held, ep) {
			missing = append(missing, ep)
		}
	}
	return missing
}

type torrentPlanCandidate struct {
	index    int
	episodes []int
	release  Torrent
}

type torrentPlanEpisodeSet [maxPlanEpisodes + 1]bool
type torrentPlanEpisodeCounts [maxPlanEpisodes + 1]int

func torrentPlanSet(episodes []int) torrentPlanEpisodeSet {
	var set torrentPlanEpisodeSet
	for _, ep := range episodes {
		set[ep] = true
	}
	return set
}

func torrentPlanKnownSize(size float64) bool {
	return size > 0 && !math.IsNaN(size) && !math.IsInf(size, 0)
}

// summarize counts episode sets, not file copies or occurrences within one
// release. The repeated, overlap and extra dimensions can intersect.
func (plan *torrentPlan) summarize(selected []torrentPlanCandidate) {
	var counts torrentPlanEpisodeCounts
	for _, candidate := range selected {
		plan.batch = append(plan.batch, candidate.index)
		plan.addRisk(candidate.release)
		for _, ep := range candidate.episodes {
			counts[ep]++
		}
	}
	plan.summarizeEpisodes(counts)
}

func (plan *torrentPlan) addRisk(release Torrent) {
	if torrentPlanKnownSize(release.Size) {
		plan.totalSize += release.Size
	} else {
		plan.unknownSizes++
	}
	if release.HitAndRun {
		plan.hitAndRun++
	}
	if release.Seeders <= 0 {
		plan.unseeded++
	}
}

func (plan *torrentPlan) summarizeEpisodes(counts torrentPlanEpisodeCounts) {
	for ep := 1; ep <= plan.scope.count; ep++ {
		if counts[ep] == 0 {
			continue
		}
		if counts[ep] > 1 {
			plan.overlap = append(plan.overlap, ep)
		}
		if slices.Contains(plan.scope.held, ep) {
			plan.repeated = append(plan.repeated, ep)
		}
		if ep < plan.scope.from {
			plan.extra = append(plan.extra, ep)
		}
	}
	for _, ep := range plan.missing {
		if counts[ep] > 0 {
			plan.covered = append(plan.covered, ep)
		} else {
			plan.remaining = append(plan.remaining, ep)
		}
	}
}
