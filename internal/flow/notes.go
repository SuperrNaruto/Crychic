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
// pick as it was. MoviePilot answers an upstream failure with an empty
// result, so a pick still without synopsis is looked up again the next
// time its page shows. Every lookup of a page runs at once, so a page
// costs one round trip, not one per pick or per kind of lookup.
func (e *Engine) annotate(ctx context.Context, sess *session, first, end int) {
	if len(sess.noted) != len(sess.picks) {
		sess.picks, sess.noted = slices.Clone(sess.picks), make([]bool, len(sess.picks))
	}
	picks, noted := sess.picks, sess.noted
	var wg sync.WaitGroup
	for i := first; i < end; i++ {
		if noted[i] {
			continue
		}
		wg.Go(func() {
			picks[i] = e.note(ctx, picks[i])
			noted[i] = picks[i].Overview != ""
		})
	}
	wg.Wait()
}

// note looks pick up on TMDB by its title and its original title, and in
// its own source for a synopsis, all at once; the TMDB twin found by title
// wins over the one found by original title.
func (e *Engine) note(ctx context.Context, pick Media) Media {
	var byTitle, byOriginal []Media
	var own Details
	var wg sync.WaitGroup
	wg.Go(func() { byTitle = e.lookup(ctx, pick.Title) })
	if original := pick.OriginalTitle; original != "" && original != pick.Title {
		wg.Go(func() { byOriginal = e.lookup(ctx, original) })
	}
	if pick.Overview == "" {
		wg.Go(func() { own = e.details(ctx, pick) })
	}
	wg.Wait()
	m, ok := tmdbTwin(pick, byTitle)
	if !ok {
		m, _ = tmdbTwin(pick, byOriginal)
	}
	if m.Overview == "" {
		m.Overview = own.Overview
	}
	return m
}

// lookup searches term; a failure is logged and finds nothing.
func (e *Engine) lookup(ctx context.Context, term string) []Media {
	results, err := e.backend.Search(ctx, term)
	if err != nil {
		e.log.Warn("calendar lookup unavailable", "title", term, "err", err)
	}
	return results
}

// tmdbTwin is the one TMDB result that is m under either of its titles,
// the same year and kind, keeping the calendar's air date and its own
// rating, if any; m itself when there is none or more than one. The TMDB
// title is kept too, so a later pick searches by it and matches by id.
func tmdbTwin(m Media, results []Media) (Media, bool) {
	var twin *Media
	for i, r := range results {
		if !r.twinOf(m) {
			continue
		}
		if twin != nil {
			return m, false
		}
		twin = &results[i]
	}
	if twin == nil {
		return m, false
	}
	t := *twin
	t.Released = m.Released
	if m.Rating > 0 {
		t.Rating = m.Rating
	}
	return t, true
}

func (r Media) twinOf(m Media) bool {
	if r.Source != tmdbSource || r.Year != m.Year || r.Kind != m.Kind {
		return false
	}
	return r.Title == m.Title || r.Title == m.OriginalTitle || (m.OriginalTitle != "" && r.OriginalTitle == m.OriginalTitle)
}
