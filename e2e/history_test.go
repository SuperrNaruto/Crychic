package e2e

import (
	"net/http"
	"testing"
)

const (
	tvHistory    = "GET /api/v1/subscribe/history/电视剧"
	movieHistory = "GET /api/v1/subscribe/history/电影"
	mujicaLookup = "GET /api/v1/subscribe/media/274580"
	blueLookup   = "GET /api/v1/subscribe/media/79166"
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

// A past subscription numbered by an episode group is asked about as that
// group's season, so the standard season's subscription does not stand in
// for it.
func TestResubscribeGroupSeasonBesideStandardOne(t *testing.T) {
	past := map[string]any{
		"id": 9, "name": "鬼灭之刃", "year": "2019", "type": "电视剧", "season": 2, "start_episode": 0,
		"media_source": "themoviedb", "media_id": "85937", "date": "2026-10-06 22:36:35",
		"episode_group": "66c6dcd6cd3639e51ad7ec9f",
	}
	h := start(t, scenario{routes: map[string]route{
		subsPath: ok(standardKimetsuSeason2(t)), tvHistory: ok(writeFixture(t, []any{past})),
		kimetsuLookup: ok(writeFixture(t, kimetsuSeason2(t, nil))),
	}})
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 1, "订阅历史")
	h.tap(alice, 1, "1")
	h.shows(1, "要重新订阅《鬼灭之刃》第 2 季（按剧集组编排）吗？")
	h.tr.verify(t)
}

// Older history is read a page at a time and appended under its own
// numbers. A record without a media identity is counted but not listed and
// still makes its page full; one a newly finished subscription pushes onto
// the next page is listed once; the last short page ends the reading. An
// older record is posted back as MoviePilot recorded it.
func TestBrowseOlderHistory(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		subsPath:      ok("subscriptions.json"),
		tvHistory:     ok("subscribe_history_tv_long.json"),
		mygoLookup:    ok("subscription_existing.json"),
		blueLookup:    ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 1, "订阅历史")
	h.shows(1, "共 12 条，已列出 9 条")
	h.mp.setRoute(tvHistory, ok("subscribe_history_tv_grown.json"))
	h.tap(alice, 1, "更多历史")
	h.shows(1, "共 13 条，已列出 11 条")
	h.tap(alice, 1, "10")
	h.tap(alice, 1, "11")
	h.tap(alice, 1, "确认重新订阅")
	h.shows(1, "已经帮你重新订阅《碧蓝之海》第 1 季（从第 5 集开始）啦")
	h.tap(alice, 1, "返回")
	h.shows(1, "订阅历史 · 电视剧")
	h.tr.verify(t)
}
