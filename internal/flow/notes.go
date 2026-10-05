package flow

import (
	"context"
	"slices"
	"sync"
)

const tmdbSource = "themoviedb"

// annotate gives the picks in [first, end) a TMDB identity and link where
// TMDB has the same show, and a synopsis from TMDB, else from their own
// source. Calendar picks carry Bangumi ids and no synopsis, while transfer
// history only carries TMDB ids. It is cosmetic: a failed lookup leaves the
// pick as it was. Each pass runs its lookups at once and waits for them, so
// a page costs a few round trips, not one per pick.
func (e *Engine) annotate(ctx context.Context, sess *session, first, end int) {
	if len(sess.noted) != len(sess.picks) {
		sess.picks, sess.noted = slices.Clone(sess.picks), make([]bool, len(sess.picks))
	}
	var open []int
	for i := first; i < end; i++ {
		if !sess.noted[i] {
			sess.noted[i] = true
			open = append(open, i)
		}
	}
	own, picks := slices.Clone(sess.picks), sess.picks
	left := inParallel(open, func(i int) bool { return e.tmdbTwin(ctx, &picks[i], picks[i].Title) })
	inParallel(left, func(i int) bool {
		original := picks[i].OriginalTitle
		return original != "" && original != picks[i].Title && e.tmdbTwin(ctx, &picks[i], original)
	})
	inParallel(open, func(i int) bool {
		if picks[i].Overview == "" {
			picks[i].Overview = e.details(ctx, own[i]).Overview
		}
		return true
	})
}

// inParallel runs done on each open index at once and returns those it
// did not finish.
func inParallel(open []int, done func(int) bool) []int {
	finished := make([]bool, len(open))
	var wg sync.WaitGroup
	for j, i := range open {
		wg.Add(1)
		go func() {
			defer wg.Done()
			finished[j] = done(i)
		}()
	}
	wg.Wait()
	var left []int
	for j, i := range open {
		if !finished[j] {
			left = append(left, i)
		}
	}
	return left
}

// tmdbTwin searches term and, when exactly one TMDB result is m under
// either of its titles, the same year and kind, makes m that result while
// keeping the calendar's air date and its own rating, if any. The TMDB
// title is kept too, so a later pick searches by it and matches by id.
func (e *Engine) tmdbTwin(ctx context.Context, m *Media, term string) bool {
	results, err := e.backend.Search(ctx, term)
	if err != nil {
		e.log.Warn("calendar lookup unavailable", "title", term, "err", err)
		return false
	}
	var twin *Media
	for i, r := range results {
		if !r.twinOf(*m) {
			continue
		}
		if twin != nil {
			return false
		}
		twin = &results[i]
	}
	if twin == nil {
		return false
	}
	t := *twin
	t.Released = m.Released
	if m.Rating > 0 {
		t.Rating = m.Rating
	}
	*m = t
	return true
}

func (r Media) twinOf(m Media) bool {
	if r.Source != tmdbSource || r.Year != m.Year || r.Kind != m.Kind {
		return false
	}
	return r.Title == m.Title || r.Title == m.OriginalTitle || (m.OriginalTitle != "" && r.OriginalTitle == m.OriginalTitle)
}
