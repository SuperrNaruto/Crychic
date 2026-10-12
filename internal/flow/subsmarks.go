package flow

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// defaultSubsGrace is how long a finished search or season read waits
	// for the subscription list; the marks are cosmetic and never hold a
	// reply back for longer.
	defaultSubsGrace = 300 * time.Millisecond

	markSubscribed = "已订阅"
	markPaused     = "已暂停"
)

// subsRead is a subscription-list read running beside other reads.
type subsRead struct {
	done  chan []Subscription
	grace time.Duration
}

// startSubs reads the subscription list in the background, bounded by the
// cosmetic timeout and the request's own context; Engine.Wait waits for it. A failed read yields no
// list, so no marks show.
func (e *Engine) startSubs(ctx context.Context) *subsRead {
	r := &subsRead{done: make(chan []Subscription, 1), grace: e.subsGrace}
	e.work.Go(func() {
		ctx, cancel := context.WithTimeout(ctx, cosmeticTimeout)
		defer cancel()
		subs, err := e.backend.Subscriptions(ctx)
		if err != nil {
			e.log.Warn("subscription marks unavailable", "err", err)
			subs = nil
		}
		r.done <- subs
	})
	return r
}

// wait returns the list if it is ready within the grace period, else nil.
func (r *subsRead) wait(ctx context.Context) []Subscription {
	if r == nil {
		return nil
	}
	timer := time.NewTimer(r.grace)
	defer timer.Stop()
	select {
	case subs := <-r.done:
		return subs
	case <-timer.C:
	case <-ctx.Done():
	}
	return nil
}

func sameTitle(s Subscription, m Media) bool {
	return s.Source == m.Source && s.MediaID == m.ID && s.Kind == m.Kind
}

// seasonMarks maps the media's standard seasons to 已订阅 or 已暂停;
// episode-group subscriptions count in their own numbering and are left out.
func seasonMarks(subs []Subscription, m Media) map[int]string {
	marks := map[int]string{}
	for _, s := range subs {
		if !sameTitle(s, m) || s.Season == nil || s.EpisodeGroup != "" {
			continue
		}
		mark := markSubscribed
		if s.State == statePaused {
			mark = markPaused
		}
		if marks[*s.Season] != markSubscribed {
			marks[*s.Season] = mark
		}
	}
	return marks
}

// resultMark is the tag of a search result: 已订阅 or 已暂停 for a movie,
// e.g. 已订阅第 1、2 季 for a show; empty when nothing is subscribed.
func resultMark(subs []Subscription, m Media) string {
	if m.Kind == Movie {
		return movieMark(subs, m)
	}
	marks := seasonMarks(subs, m)
	var parts []string
	for _, mark := range []string{markSubscribed, markPaused} {
		if names := seasonsNamed(marks, mark); names != "" {
			parts = append(parts, mark+names)
		}
	}
	return strings.Join(parts, "，")
}

func movieMark(subs []Subscription, m Media) string {
	mark := ""
	for _, s := range subs {
		if !sameTitle(s, m) {
			continue
		}
		if s.State != statePaused {
			return markSubscribed
		}
		mark = markPaused
	}
	return mark
}

// seasonsNamed lists the seasons carrying mark: 第 1、2 季, with 特别篇 last.
func seasonsNamed(marks map[int]string, mark string) string {
	var numbers []string
	specials := false
	for _, n := range slices.Sorted(func(yield func(int) bool) {
		for n := range marks {
			if !yield(n) {
				return
			}
		}
	}) {
		switch {
		case marks[n] != mark:
		case n == 0:
			specials = true
		default:
			numbers = append(numbers, strconv.Itoa(n))
		}
	}
	name := ""
	if len(numbers) > 0 {
		name = "第 " + strings.Join(numbers, "、") + " 季"
	}
	if specials {
		name = strings.Trim(name+"、特别篇", "、")
	}
	return name
}
