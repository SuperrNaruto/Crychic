package e2e

import (
	"time"
)

const (
	ongoingLastEpisode = 1216
	ongoingNextEpisode = 1217
	airingWeek         = 7 * 24 * time.Hour
	notifySubscription = "GET /api/v1/subscribe/1"
)

// A live season can grow after the last activity check. A failed metadata
// read must not consume its arrival, and a restart must preserve the new scope.

func growingSubscription() map[string]any {
	return map[string]any{
		"id": 1, "name": conan.title, "year": conan.year, "type": "电视剧",
		"media_source": "themoviedb", "media_id": conan.id, "season": 1,
		"state": "R", "total_episode": ongoingNextEpisode, "lack_episode": 1, "start_episode": ongoingLastEpisode,
	}
}

func setNotifySubscription(h *harness, sub map[string]any) {
	h.t.Helper()
	h.mp.setRoute(subsPath, ok(writeFixture(h.t, []map[string]any{sub})))
	h.mp.setRoute(notifySubscription, ok(writeFixture(h.t, sub)))
}

// Upgrade subscriptions remain controllable after a complete first arrival.
// New versions of the same episodes notify, even after restart and row closure.

// A delivered movie whose subscription closed waits for nothing more: a
// stuck download of the same movie is not its requester's, and a new
// upgrade subscription reusing the closed row's id belongs to its new
// requester alone.

// A season told complete stays complete once its subscription closed: a
// stray later episode numbered past its total is not announced again.
