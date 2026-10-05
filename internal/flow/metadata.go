package flow

import (
	"context"
	"sync"
	"time"
)

const cosmeticTimeout = 5 * time.Second

func (e *Engine) enrich(ctx context.Context, sess *session, media Media) {
	ctx, cancel := context.WithTimeout(ctx, cosmeticTimeout)
	defer cancel()
	var details Details
	var library Library
	var wg sync.WaitGroup
	wg.Go(func() { details = e.details(ctx, media) })
	wg.Go(func() { library = e.library(ctx, media) })
	wg.Wait()
	sess.picked, sess.library = card{Media: media, Details: details}, library
}

func (e *Engine) confirmationInfo(ctx context.Context, sess *session, target Target) (int, error) {
	cosmetic, cancel := context.WithTimeout(ctx, cosmeticTimeout)
	defer cancel()
	var downloads []Download
	var existing int
	var err error
	var wg sync.WaitGroup
	wg.Go(func() { downloads = e.downloads(cosmetic, target) })
	wg.Go(func() { existing, err = e.backend.FindSubscription(ctx, target) })
	wg.Wait()
	sess.picked.Downloads = downloads
	return existing, err
}
