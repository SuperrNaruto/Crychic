package flow

import (
	"context"
	"fmt"
	"slices"
	"sync"
)

type torrentPlanRead struct {
	library               Library
	seasons               []Season
	libraryErr, seasonErr error
}

// Both external reads share a bounded deadline and start independently.
// Successful HTTP reads describe MoviePilot's observation, not server health.
func (e *Engine) readPlanInventory(ctx context.Context, media Media) torrentPlanRead {
	ctx, cancel := context.WithTimeout(ctx, cosmeticTimeout)
	defer cancel()
	var read torrentPlanRead
	var wg sync.WaitGroup
	wg.Go(func() { read.library, read.libraryErr = e.backend.Library(ctx, media) })
	wg.Go(func() { read.seasons, read.seasonErr = e.backend.Seasons(ctx, media) })
	wg.Wait()
	return read
}

func (e *Engine) readTorrentPlan(ctx context.Context, sess session) (session, *torrentPlan) {
	read := e.readPlanInventory(ctx, sess.focus.Media)
	sess.library, sess.seasons = read.library, read.seasons
	sess.picked.LibraryUnknown = read.libraryErr != nil
	if problem := read.problem(); problem != "" {
		return sess, &torrentPlan{problem: problem}
	}
	scope, problem := read.scope(*sess.focus)
	if problem != "" {
		return sess, &torrentPlan{problem: problem}
	}
	return sess, suggestTorrentPlan(sess, scope)
}

func (read torrentPlanRead) problem() string {
	var problems []string
	if read.libraryErr != nil || read.library.Episodes == nil {
		problems = append(problems, "没能读取媒体库，不能把未知库存当成缺集哦。")
	}
	if read.seasonErr != nil {
		problems = append(problems, "没能读取季信息，暂时不能确认所需范围哦。")
	}
	return joinNonEmpty(" ", problems...)
}

func (read torrentPlanRead) scope(target Target) (torrentPlanScope, string) {
	scope := torrentPlanScope{from: max(target.StartEpisode, 1)}
	matches := 0
	for _, season := range read.seasons {
		if season.Number == *target.Season {
			scope.count = season.EpisodeCount
			matches++
		}
	}
	if matches != 1 || !scope.valid() {
		return scope, fmt.Sprintf("季集范围不确定；需要明确的起始集和季总集数，且一季最多支持 %d 集，请先在 MoviePilot 核对哦。", maxPlanEpisodes)
	}
	scope.held = slices.Clone(read.library.Episodes[*target.Season])
	if slices.ContainsFunc(scope.held, func(ep int) bool { return ep < 1 || ep > scope.count }) {
		return scope, "媒体库季集范围不确定，与本次季总集数不一致，请先在 MoviePilot 核对哦。"
	}
	slices.Sort(scope.held)
	scope.held = slices.Compact(scope.held)
	return scope, ""
}
