package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func breakingSeason(id, number int) map[string]any {
	return map[string]any{
		"id": id, "name": "绝命毒师", "year": "2008", "type": "电视剧",
		"media_source": "themoviedb", "media_id": "1396", "season": number,
		"state": "R", "total_episode": 13, "lack_episode": 13,
		"date": "2026-10-05 10:00:00",
	}
}

func requestedBreakingSeasons(t *testing.T) *harness {
	t.Helper()
	subs := []map[string]any{breakingSeason(1, 2), breakingSeason(2, 3)}
	h := start(t, scenario{routes: map[string]route{
		searchPath:                   ok("search_breaking_bad.json"),
		breakDetails:                 ok("detail_breaking_bad.json"),
		seasonsPath:                  ok("seasons_breaking_bad.json"),
		breakingQuery:                ok("subscription_none.json"),
		subscribePath:                ok("subscribe_created.json"),
		subsPath:                     ok(writeFixture(t, subs)),
		deleteFirst:                  ok("subscribe_deleted.json"),
		"DELETE /api/v1/subscribe/2": ok("subscribe_deleted.json"),
	}})
	for i, n := range []int{2, 3} {
		h.say(alice, alice, "/search 绝命毒师")
		h.tap(alice, i+1, "1")
		h.tap(alice, i+1, fmt.Sprintf("第 %d 季", n))
		h.tap(alice, i+1, "从第 1 集开始")
	}
	return h
}

// subscriptionButtons leaves the cancel confirmation on screen, returning
// the previously shown pause and cancel buttons for stale-button checks.
func subscriptionButtons(h *harness, message int, pick string) (string, string) {
	h.t.Helper()
	h.tap(alice, message, pick)
	pause, _ := findButton(mustMessage(h, message).rows, "暂停订阅")
	h.tap(alice, message, "返回")
	h.tap(alice, message, "取消订阅")
	h.tap(alice, message, pick)
	cancel, _ := findButton(mustMessage(h, message).rows, "确认取消")
	if pause == "" || cancel == "" {
		h.t.Fatal("the requested subscription must offer pause and cancel buttons")
	}
	return pause, cancel
}

func subscriptionWrites(h *harness) int {
	text := h.tr.String()
	return strings.Count(text, "-> MoviePilot PUT /api/v1/subscribe/status/") +
		strings.Count(text, "-> MoviePilot DELETE /api/v1/subscribe/")
}

func tapWithoutSubscriptionWrite(h *harness, message int, label string) {
	h.t.Helper()
	before := subscriptionWrites(h)
	h.tap(alice, message, label)
	if subscriptionWrites(h) != before {
		h.t.Errorf("%s wrote after the subscription identity changed or could not be checked", label)
	}
}

// The displayed screen is still current when MoviePilot replaces its row:
// pause, resume and cancel must re-read its identity before writing. A new
// creation date also distinguishes a re-created subscription of the same
// season. Read failures block writes, while the original row still works.
func TestSubscriptionIdentityCheckedBeforeWrites(t *testing.T) {
	h := requestedBreakingSeasons(t)
	original := ok(writeFixture(t, []map[string]any{breakingSeason(1, 2), breakingSeason(2, 3)}))
	replaced := ok(writeFixture(t, []map[string]any{breakingSeason(1, 4), breakingSeason(2, 3)}))
	recreated := breakingSeason(1, 2)
	recreated["date"] = "2026-10-05 11:00:00"
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 3, "1")
	h.mp.setRoute(subsPath, replaced)
	tapWithoutSubscriptionWrite(h, 3, "暂停订阅")
	h.mp.setRoute(subsPath, original)
	h.tap(alice, 3, "暂停订阅")
	h.shows(3, "已暂停")
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{recreated})))
	tapWithoutSubscriptionWrite(h, 3, "恢复订阅")
	h.mp.setRoute(subsPath, original)
	h.tap(alice, 3, "恢复订阅")
	h.shows(3, "订阅中")
	h.tap(alice, 3, "返回")
	h.tap(alice, 3, "取消订阅")
	h.tap(alice, 3, "1")
	h.mp.setRoute(subsPath, replaced)
	tapWithoutSubscriptionWrite(h, 3, "确认取消")
	h.mp.setRoute(subsPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	tapWithoutSubscriptionWrite(h, 3, "确认取消")
	h.mp.setRoute(subsPath, original)
	h.tap(alice, 3, "确认取消")
	h.shows(3, "《绝命毒师》第 2 季的订阅已经取消啦")
	if subscriptionWrites(h) != 3 {
		t.Error("the original subscription must still accept pause, resume and cancel")
	}
	h.tr.verify(t)
}
