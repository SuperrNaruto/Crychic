package flow

import (
	"context"
	"slices"
)

const (
	msgSubChanged = "这条订阅已经变更或结束啦，先用 /subscribe 重新确认吧～"
	msgSubCheck   = "暂时没能核对订阅，稍后再试吧～"
)

// same identifies the original subscription, not merely its reusable row
// id. Progress and settings may change without changing its identity.
func (s Subscription) same(other Subscription) bool {
	if s.Source == "" || s.MediaID == "" {
		return false
	}
	return s.ID == other.ID && s.Source == other.Source && s.MediaID == other.MediaID &&
		s.Kind == other.Kind && s.Created == other.Created && s.EpisodeGroup == other.EpisodeGroup &&
		sameSeason(s.Season, other.Season)
}

func sameSeason(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// checkSubscription rechecks the displayed identity and ownership before
// a write. MoviePilot's mutations accept only a row id, so an unknown or
// replaced row must not reach them. A failed read leaves the screen intact.
func (e *Engine) checkSubscription(ctx context.Context, sess session, sub Subscription) string {
	subs, err := e.backend.Subscriptions(ctx)
	if err != nil {
		e.log.Warn("subscription identity unavailable", "subscription", sub.ID, "err", err)
		if message, safe := UserMessage(err); safe {
			return message
		}
		return msgSubCheck
	}
	if !slices.ContainsFunc(subs, sub.same) {
		return msgSubChanged
	}
	if !slices.Contains(e.watcher.Requested(sess.owner.UserID, []Subscription{sub}), sub.ID) {
		return msgInvalidChoice
	}
	return ""
}
