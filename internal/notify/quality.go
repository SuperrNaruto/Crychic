package notify

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"
)

// qualityBudget bounds reading one notice's release qualities; they are
// cosmetic, so a slow or failed read only leaves them out.
const qualityBudget = 10 * time.Second

// qualities words each release's quality once, in release order, reading
// them all at once.
func (n *Notifier) qualities(ctx context.Context, releases []release) []string {
	ctx, cancel := context.WithTimeout(ctx, qualityBudget)
	defer cancel()
	words := make([]string, len(releases))
	var wg sync.WaitGroup
	for i, r := range releases {
		wg.Go(func() {
			q, err := n.opts.Feed.Quality(ctx, r.File)
			if err != nil {
				n.opts.Log.Warn("release quality unavailable", "file", r.File, "err", err)
				return
			}
			words[i] = q.words()
		})
	}
	wg.Wait()
	var out []string
	for _, w := range words {
		if w != "" && !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

// words is e.g. "1080p · BluRay · x265 · FLAC 2.0 · FROGE".
func (q Quality) words() string {
	var parts []string
	for _, p := range []string{q.Resolution, q.Edition, q.WebSource, q.Video, q.Audio, q.Group} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}
