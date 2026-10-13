package flow

import (
	"context"
	"sync"
	"time"
)

const (
	cosmeticTimeout = 5 * time.Second
	// calendarCardTimeout bounds a calendar pick's card reads once the
	// calendar page has already given it a synopsis: MoviePilot's first,
	// cold lookups are slow and the card has what it needs without them.
	calendarCardTimeout = 2 * time.Second
)

// cardBudget is how long the cosmetic reads of media's card may take.
func cardBudget(media Media) time.Duration {
	if media.CalendarID != "" && media.Overview != "" {
		return calendarCardTimeout
	}
	return cosmeticTimeout
}

func (e *Engine) enrich(ctx context.Context, sess *session, media Media) {
	ctx, cancel := context.WithTimeout(ctx, cardBudget(media))
	defer cancel()
	var details Details
	var library Library
	var libraryErr error
	var wg sync.WaitGroup
	wg.Go(func() { details = e.details(ctx, media) })
	wg.Go(func() { library, libraryErr = e.library(ctx, media) })
	wg.Wait()
	sess.picked = card{Media: media, Details: details, LibraryUnknown: libraryErr != nil}
	sess.library = library
}

func (e *Engine) confirmationInfo(ctx context.Context, sess *session, target Target) (Subscription, error) {
	cosmetic, cancel := context.WithTimeout(ctx, cosmeticTimeout)
	defer cancel()
	var downloads []Download
	var existing Subscription
	var err error
	var wg sync.WaitGroup
	wg.Go(func() { downloads = e.downloads(cosmetic, target) })
	wg.Go(func() { existing, err = e.backend.FindSubscription(ctx, target) })
	wg.Wait()
	sess.picked.Downloads = downloads
	return existing, err
}
