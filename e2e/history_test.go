package e2e

import (
	"net/http"
	"strings"
	"testing"
)

const (
	tvHistory    = "GET /api/v1/subscribe/history/电视剧"
	movieHistory = "GET /api/v1/subscribe/history/电影"
	mujicaLookup = "GET /api/v1/subscribe/media/274580"
)

var mujica = show{"颂乐人偶", "2025", "274580", "https://image.tmdb.org/t/p/w500/xixekXvcCSZS1jXhFrYUzpCrLL6.jpg"}

// The history lists each kind's newest past subscriptions. One still
// subscribed is only flashed; another is posted back as MoviePilot recorded
// it, once however often its button is pressed, and its arrival is
// announced. A write with an unknown result points to /subscribe.
func TestResubscribeFromHistory(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		subsPath:      ok("subscriptions.json"),
		tvHistory:     ok("subscribe_history_tv.json"),
		movieHistory:  ok("subscribe_history_movie.json"),
		mygoLookup:    ok("subscription_existing.json"),
		mujicaLookup:  ok("subscription_none.json"),
		duneLookup:    ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 1, "订阅历史")
	h.shows(1, "订阅历史 · 电视剧")
	h.tap(alice, 1, "电影")
	h.shows(1, "《沙丘》")
	h.tap(alice, 1, "电视剧")
	h.tap(alice, 1, "5")
	h.shows(1, "订阅历史 · 电视剧")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "1")
	confirm, _ := findButton(mustMessage(h, 1).rows, "确认重新订阅")
	h.tap(alice, 1, "确认重新订阅")
	h.shows(1, "已经帮你重新订阅《颂乐人偶》第 1 季啦")
	h.tapData(alice, 1, confirm)
	h.arrives(alice, mujica.file("S01", "E01"))
	h.tap(alice, 1, "返回")
	h.shows(1, "订阅历史 · 电视剧")

	h.mp.setRoute(subscribePath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, 1, "电影")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "确认重新订阅")
	h.shows(1, "/subscribe")
	h.tr.verify(t)
}

// Returning home keeps the conversation but must not lend its previous
// show's season lengths or held episodes to a resubscription. The first
// batch is complete as a delivery, not as a season; later episodes still
// arrive after restart.
func TestResubscribeAfterAnotherShow(t *testing.T) {
	const conversation, firstArrival, finalArrival = 1, 2, 3
	h := start(t, scenario{quiet: "0s", routes: map[string]route{
		searchPath:      ok("search_breaking_bad.json"),
		breakDetails:    ok("detail_breaking_bad.json"),
		seasonsPath:     ok("seasons_breaking_bad.json"),
		breakingQuery:   ok("subscription_none.json"),
		subscribePath:   ok("subscribe_created.json"),
		subsPath:        ok(writeFixture(t, []map[string]any{breakingSeason(1, 2)})),
		tvHistory:       ok("subscribe_history_tv.json"),
		mujicaLookup:    ok("subscription_none.json"),
		libraryShowPath: ok(writeFixture(t, map[string][]int{"1": {1, 2}})),
	}})
	h.say(alice, alice, "/search 绝命毒师")
	h.tap(alice, conversation, "1")
	h.tap(alice, conversation, "第 2 季")
	h.tap(alice, conversation, "从第 1 集开始")
	h.mp.setRoute(libraryShowPath, ok(writeFixture(t, map[string][]int{})))
	h.tap(alice, conversation, "首页")
	h.tap(alice, conversation, "我的订阅")
	h.tap(alice, conversation, "订阅历史")
	h.tap(alice, conversation, "1")
	h.tap(alice, conversation, "确认重新订阅")
	h.shows(conversation, "已经帮你重新订阅《颂乐人偶》第 1 季啦")
	h.transfers(mujica.file("S01", "E01-E07"))
	first, ok := h.tg.message(firstArrival)
	if !ok || !strings.Contains(first.text, mujica.title) || !strings.Contains(first.text, "E01–E07") {
		t.Error("the resubscribed show's arrival inherited the previous show's held episodes")
	}
	if strings.Contains(first.text, "全部到齐") {
		t.Error("the resubscribed show ended at the previous show's seven-episode season length")
	}
	h.restart()
	h.transfers(mujica.file("S01", "E08-E13"))
	last, ok := h.tg.message(finalArrival)
	if !ok || !strings.Contains(last.text, "E08–E13") {
		t.Error("the resubscribed show's later episodes arrived without a notice")
	}
	h.tr.verify(t)
}
