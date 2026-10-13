package flow

import (
	"container/list"
	"context"
	"sync"
	"time"
)

const (
	// sourceBangumi is how MoviePilot names Bangumi media.
	sourceBangumi = "bangumi"
	// Bangumi subject types a title search keeps: anime and live-action.
	subjectAnime      = 2
	subjectLiveAction = 6
	// subjectBudget bounds the whole batch of type lookups of one search;
	// results whose type is not known by then stay in the list.
	subjectBudget = 1500 * time.Millisecond
	// subjectSlots is how many type lookups of one search run at once.
	subjectSlots = 4
	// subjectCacheSize bounds the remembered subject types.
	subjectCacheSize = 1024
)

// subjectCache remembers the type of Bangumi subjects, which never change;
// the least recently used are forgotten beyond subjectCacheSize.
type subjectCache struct {
	mu    sync.Mutex
	order *list.List // of subjectEntry, most recent first
	byID  map[string]*list.Element
}

type subjectEntry struct {
	id   string
	kind int
}

func newSubjectCache() *subjectCache {
	return &subjectCache{order: list.New(), byID: map[string]*list.Element{}}
}

func (c *subjectCache) get(id string) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.byID[id]
	if !ok {
		return 0, false
	}
	c.order.MoveToFront(el)
	return el.Value.(subjectEntry).kind, true
}

func (c *subjectCache) put(id string, kind int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.byID[id]; ok {
		c.order.MoveToFront(el)
		return
	}
	c.byID[id] = c.order.PushFront(subjectEntry{id: id, kind: kind})
	if c.order.Len() > subjectCacheSize {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.byID, oldest.Value.(subjectEntry).id)
	}
}

// onlyAnimeAndLive drops the Bangumi results that are books, music or games:
// a title search through MoviePilot also returns manga and novels typed as
// shows, and opening one makes its details and seasons reads fail. A result
// whose type is unknown (lookup failed, slow or missing) stays. The lookups
// run at once under one budget, and a known type is remembered.
func (e *Engine) onlyAnimeAndLive(ctx context.Context, results []Media) []Media {
	if e.calendar == nil {
		return results
	}
	kinds := e.subjectKinds(ctx, results)
	kept := make([]Media, 0, len(results))
	for _, m := range results {
		if kind := kinds[m.ID]; m.Source == sourceBangumi && kind != 0 && kind != subjectAnime && kind != subjectLiveAction {
			continue
		}
		kept = append(kept, m)
	}
	if dropped := len(results) - len(kept); dropped > 0 {
		e.log.Info("dropped Bangumi results that are not anime", "dropped", dropped)
	}
	return kept
}

// subjectKinds maps each Bangumi result's ID to its known type; an unknown
// one is absent.
func (e *Engine) subjectKinds(ctx context.Context, results []Media) map[string]int {
	kinds := map[string]int{}
	var missing []string
	for _, m := range results {
		if m.Source != sourceBangumi {
			continue
		}
		if kind, ok := e.subjects.get(m.ID); ok {
			kinds[m.ID] = kind
		} else if _, queued := kinds[m.ID]; !queued {
			kinds[m.ID] = 0
			missing = append(missing, m.ID)
		}
	}
	if len(missing) == 0 {
		return kinds
	}
	ctx, cancel := context.WithTimeout(ctx, subjectBudget)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, subjectSlots)
	for _, id := range missing {
		wg.Go(func() {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				return
			}
			kind, err := e.calendar.SubjectType(ctx, id)
			if err != nil || kind == 0 {
				return
			}
			e.subjects.put(id, kind)
			mu.Lock()
			kinds[id] = kind
			mu.Unlock()
		})
	}
	wg.Wait()
	return kinds
}
