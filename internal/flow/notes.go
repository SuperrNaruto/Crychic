package flow

import (
	"context"
	"slices"
	"sync"
	"time"
)

const (
	tmdbSource = "themoviedb"
	// noteMPSlots is how many MoviePilot calls note lookups make at once,
	// across all pages and rounds.
	noteMPSlots = 2
)

// A calendar page gives its picks a TMDB identity and link where TMDB has
// the same show, and a synopsis from TMDB, else from the calendar itself.
// Calendar picks carry Bangumi ids and no synopsis, while transfer history
// only carries TMDB ids. It is cosmetic: lookups run in background rounds
// that remember what they found (noteCache), and a page shows what is known
// after waiting the grace; the rest follows through Reply.Follow.

// annotate gives the picks of the page at the notes remembered for them. With
// start, picks without a fresh one are looked up first, waiting at most the
// grace. The result closes when lookups still running for the page finish,
// nil when none are.
func (e *Engine) annotate(ctx context.Context, sess *session, at span, start bool) <-chan struct{} {
	sess.picks = slices.Clone(sess.picks)
	window := sess.picks[at.first:at.end]
	if start {
		e.awaitNotes(ctx, e.startNotes(window))
	}
	now := e.now()
	for i, p := range window {
		if m, ok := e.notes.apply(p, now); ok {
			window[i] = m
		}
	}
	return e.joined(e.notes.active(window, now))
}

// startNotes looks up the picks without a fresh note in one background
// round, leaving those another round is on; it returns every round covering
// them.
func (e *Engine) startNotes(picks []Media) []*noteRound {
	cl := e.notes.claim(picks, e.now())
	if len(cl.mine) > 0 {
		e.work.Go(func() { e.runNotes(cl.round, cl.mine) })
	}
	return cl.covering
}

// awaitNotes waits for the rounds, at most the grace.
func (e *Engine) awaitNotes(ctx context.Context, rounds []*noteRound) {
	done := e.joined(rounds)
	if done == nil {
		return
	}
	timer := time.NewTimer(e.noteGrace)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	case <-ctx.Done():
	}
}

// joined closes when every round has finished; nil when there are none.
func (e *Engine) joined(rounds []*noteRound) <-chan struct{} {
	switch len(rounds) {
	case 0:
		return nil
	case 1:
		return rounds[0].done
	}
	all := make(chan struct{})
	e.work.Go(func() {
		for _, r := range rounds {
			<-r.done
		}
		close(all)
	})
	return all
}

// runNotes looks up picks under one cosmetic deadline of the app's own run,
// so a page closing or a press does not stop what later pages will reuse.
// Each pick is remembered as soon as its lookup ends; done closes with the
// last of them.
func (e *Engine) runNotes(round *noteRound, picks []Media) {
	ctx, cancel := context.WithTimeout(e.runCtx, cosmeticTimeout)
	defer cancel()
	defer close(round.done)
	e.lookupNotes(ctx, picks)
}

// lookupNotes reads MoviePilot for every pick at once, then the calendar
// itself for the synopses still missing, preserving provider preference
// without extending the common deadline. A pick whose lookup ended before
// the deadline is done, whether or not it found a synopsis.
func (e *Engine) lookupNotes(ctx context.Context, picks []Media) {
	results := make([]noteResult, len(picks))
	var wg sync.WaitGroup
	for i, p := range picks {
		wg.Go(func() {
			m := e.note(ctx, p)
			results[i] = noteResult{pick: m, done: m.Overview != "" && ctx.Err() == nil}
			if results[i].done {
				e.notes.settle(results[i], e.now())
			}
		})
	}
	wg.Wait()
	for i := range results {
		if results[i].done {
			continue
		}
		wg.Go(func() { e.settleSummary(ctx, &results[i]) })
	}
	wg.Wait()
}

// budgetEnded tells whether the round's own deadline, not the app's
// shutdown, ended ctx.
func (e *Engine) budgetEnded(ctx context.Context) bool {
	return ctx.Err() != nil && e.runCtx.Err() == nil
}

// settleSummary asks the calendar itself for the synopsis r still lacks.
func (e *Engine) settleSummary(ctx context.Context, r *noteResult) {
	if ctx.Err() == nil {
		r.pick.Overview = e.summary(ctx, r.pick)
		r.done = ctx.Err() == nil
		r.miss = r.pick.Overview == ""
	}
	r.timedOut = !r.done && e.budgetEnded(ctx)
	e.notes.settle(*r, e.now())
}

// Independent TMDB searches and MoviePilot's Bangumi synopsis overlap. Direct
// Bangumi is only consulted if neither yields a synopsis within the page budget.
func (e *Engine) note(ctx context.Context, pick Media) Media {
	var byTitle, byOriginal []Media
	own := pick.Overview
	var summaryErr error
	var wg sync.WaitGroup
	wg.Go(func() { byTitle = e.lookup(ctx, pick.Title) })
	if original := pick.OriginalTitle; original != "" && original != pick.Title {
		wg.Go(func() { byOriginal = e.lookup(ctx, original) })
	}
	if pick.Overview == "" {
		wg.Go(func() { own, summaryErr = e.mpSummary(ctx, pick.CalendarID) })
	}
	wg.Wait()
	m, ok := tmdbTwin(pick, byTitle)
	if !ok {
		m, _ = tmdbTwin(pick, byOriginal)
	}
	if m.Overview == "" {
		m.Overview = own
	}
	if summaryErr != nil && ctx.Err() == nil {
		e.log.Warn("backend calendar synopsis unavailable", "pick", pick.CalendarID, "err", summaryErr)
	}
	return m
}

// summary is pick's synopsis from the calendar itself; a failure is
// logged and leaves it without one, to be looked up again.
func (e *Engine) summary(ctx context.Context, pick Media) string {
	s, err := e.calendar.Summary(ctx, pick.CalendarID)
	if err != nil && ctx.Err() == nil {
		e.log.Warn("calendar synopsis unavailable", "pick", pick.CalendarID, "err", err)
	}
	return s
}

// mpSummary is MoviePilot's Bangumi synopsis of id, once a slot is free.
func (e *Engine) mpSummary(ctx context.Context, id string) (string, error) {
	if !e.takeSlot(ctx) {
		return "", ctx.Err()
	}
	defer e.freeSlot()
	return e.backend.BangumiSummary(ctx, id)
}

// takeSlot waits for one of the few MoviePilot calls all note lookups share,
// and gives up when ctx ends; its media search is costly for MoviePilot and
// goes on after the client stops waiting.
func (e *Engine) takeSlot(ctx context.Context) bool {
	select {
	case e.mpSlots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (e *Engine) freeSlot() { <-e.mpSlots }

// lookup searches term; a failure is logged and finds nothing.
func (e *Engine) lookup(ctx context.Context, term string) []Media {
	if !e.takeSlot(ctx) {
		return nil
	}
	defer e.freeSlot()
	results, err := e.backend.Search(ctx, term)
	if err != nil && ctx.Err() == nil {
		e.log.Warn("calendar lookup unavailable", "title", term, "err", err)
	}
	return results
}

// tmdbTwin is the one TMDB result that is m under either of its titles,
// the same year and kind, keeping the calendar's air date, weekday, id and
// its own rating, if any; m itself when there is none or more than one. The TMDB
// title is kept too, so a later pick searches by it and matches by id; a
// sequel keeps its own name (青之箱 第二季), whose search names its season.
func tmdbTwin(m Media, results []Media) (Media, bool) {
	i, ok := twinIndex(m, results)
	if !ok {
		return m, false
	}
	t := results[i]
	t.Released, t.Weekday, t.CalendarID = m.Released, m.Weekday, m.CalendarID
	if t.Season > 0 {
		t.Title = m.Title
	}
	if m.Rating > 0 {
		t.Rating = m.Rating
	}
	return t, true
}

// twinIndex finds the one TMDB result that twins m; none when there is no
// such result or more than one.
func twinIndex(m Media, results []Media) (int, bool) {
	found := -1
	for i, r := range results {
		if !r.twinOf(m) {
			continue
		}
		if found >= 0 {
			return 0, false
		}
		found = i
	}
	return found, found >= 0
}

func (r Media) twinOf(m Media) bool {
	if r.Source != tmdbSource || r.Kind != m.Kind || !r.sameRun(m) {
		return false
	}
	return r.searched() == m.Title || r.Title == m.Title || r.Title == m.OriginalTitle ||
		(m.OriginalTitle != "" && r.OriginalTitle == m.OriginalTitle)
}

// searched is r's title as MoviePilot's search named it, with the season
// the search term named.
func (r Media) searched() string {
	if r.Season > 0 {
		return r.Title + SeasonSuffix(r.Season)
	}
	return r.Title
}

// sameRun tells whether a TMDB result r can be calendar pick m by its year.
// TMDB dates a show by its first season, while the calendar lists a later
// season (青之箱 第二季) as its own show of the year it airs; a result
// for a later season named in the search term may therefore start earlier.
func (r Media) sameRun(m Media) bool {
	if r.Year == m.Year {
		return true
	}
	return r.Season > 1 && r.Year != "" && r.Year < m.Year
}
