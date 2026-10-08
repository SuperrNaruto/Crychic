package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
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
func TestOngoingSubscriptionKeepsNewEpisodeNotifications(t *testing.T) {
	const firstNotice, finalNotice = 2, 4
	h := start(t, scenario{routes: conanRoutes(), quiet: "0s", stall: "0s"})
	h.say(alice, alice, "/search 名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.tap(alice, 1, conanFirst)
	h.tap(alice, 1, "只追新集（第 1216 集起）")
	h.advance(airingWeek)
	h.transfers()
	h.tr.add(">> The ongoing season gains episode 1217; its subscription read is temporarily unavailable")
	h.mp.setRoute(notifySubscription, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.transfers(conan.file("S01", fmt.Sprintf("E%d", ongoingLastEpisode)))
	noticeCount(h, 0)
	h.restart()
	h.tr.add(">> MoviePilot exposes the same active subscription with its new episode count")
	setNotifySubscription(h, growingSubscription())
	h.transfers()
	noticeAt(h, arrivalCheck{message: firstNotice, chat: alice, title: "E1216 到家啦"})
	if strings.Contains(mustMessage(h, firstNotice).text, "全部到齐") {
		t.Error("a growing season was declared complete at its original episode count")
	}
	h.say(alice, alice, "/subscribe")
	if !strings.Contains(mustMessage(h, 3).text, "你请求的") {
		t.Error("the still-active subscription lost its requester")
	}
	h.advance(airingWeek)
	h.transfers()
	h.tr.add(">> MoviePilot finishes the subscription after scheduling episode 1217, before it transfers")
	h.mp.setRoute(notifySubscription, ok("subscription_none.json"))
	h.mp.setRoute(subsPath, ok("subscriptions_none.json"))
	h.transfers(conan.file("S01", fmt.Sprintf("E%d", ongoingNextEpisode)))
	noticeAt(h, arrivalCheck{message: finalNotice, chat: alice, title: "E1217 到家啦"})
	h.transfers()
	noticeCount(h, 2)
	h.tr.verify(t)
}

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
func TestUpgradeSubscriptionKeepsControlsAndNotifiesNewVersions(t *testing.T) {
	const details, firstNotice, secondNotice, finalNotice = 2, 3, 4, 5
	sub := breakingSeason(1, 2)
	sub["best_version"] = 1
	h := start(t, scenario{quiet: "0s", routes: map[string]route{
		searchPath: ok("search_breaking_bad.json"), breakDetails: ok("detail_breaking_bad.json"),
		seasonsPath: ok("seasons_breaking_bad.json"), breakingQuery: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 绝命毒师")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 2 季")
	h.tap(alice, 1, "从第 1 集开始")
	setNotifySubscription(h, sub)
	h.say(alice, alice, "/subscribe")
	h.tap(alice, details, "1")
	h.shows(details, "洗版订阅")
	h.tr.add(">> MoviePilot keeps this upgrade subscription active for a better release")
	h.transfers(breakingBad.file("S02", "E01-E13"))
	noticeAt(h, arrivalCheck{message: firstNotice, chat: alice, title: "E01–E13 到家啦"})
	h.tap(alice, details, "暂停订阅")
	if subscriptionWrites(h) == 0 {
		t.Error("the active upgrade subscription lost its requester's pause permission")
	}
	if _, found := findButton(mustMessage(h, details).rows, "恢复订阅"); found {
		h.tap(alice, details, "恢复订阅")
	} else {
		t.Error("the requester cannot resume the paused upgrade subscription")
	}
	h.tap(alice, details, "返回")
	if _, found := findButton(mustMessage(h, details).rows, "取消订阅"); !found {
		t.Error("the requester cannot cancel the active upgrade subscription")
	}
	h.restart()
	h.transfers(breakingBad.file("S02", "E01"))
	noticeAt(h, arrivalCheck{message: secondNotice, chat: alice, title: "E01 到家啦"})
	h.mp.setRoute(notifySubscription, ok("subscription_none.json"))
	h.transfers(breakingBad.file("S02", "E02"))
	noticeAt(h, arrivalCheck{message: finalNotice, chat: alice, title: "E02 到家啦"})
	h.restart()
	h.transfers()
	noticeCount(h, 3)
	if strings.Contains(h.tr.String(), "这一季你要的剧集全部到齐") {
		t.Error("ordinary delivered episodes were treated as proof that an upgrade is complete")
	}
	h.tr.verify(t)
}
