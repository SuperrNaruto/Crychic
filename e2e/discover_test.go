package e2e

import "testing"

const (
	diggerDetails = "GET /api/v1/media/1248832"
	diggerLookup  = "GET /api/v1/subscribe/media/1248832"
	abyssDetails  = "GET /api/v1/media/301489"
	trendingPath  = "GET /api/v1/recommend/tmdb_trending"
	doubanTVPath  = "GET /api/v1/recommend/douban_tv_hot"
	animePath     = "GET /api/v1/recommend/bangumi_calendar"
)

// A trending TMDB pick is found again by search and goes straight to its
// card, ready to subscribe.
func TestTrendingPickGoesStraightToSubscribe(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		trendingPath:  ok("chart_trending.json"),
		searchPath:    ok("search_digger.json"),
		diggerDetails: ok("detail_digger.json"),
		diggerLookup:  ok("subscription_none.json"),
	}})
	h.say(alice, alice, "/hot")
	h.tap(alice, 1, "🔥 TMDB 流行趋势")
	h.tap(alice, 1, "1. 挖掘者 (2026)")
	h.shows(1, "确认订阅《挖掘者》")
	h.tr.verify(t)
}

// Douban picks carry Douban ids, so they are matched by title: a sure
// match goes on to its seasons, an unsure one lets the user choose.
func TestDoubanPicksAreMatchedByTitle(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		doubanTVPath: ok("chart_douban_tv.json"),
		searchPath:   ok("search_abyss.json"),
		abyssDetails: ok("detail_abyss.json"),
		seasonsPath:  ok("seasons_abyss.json"),
	}})
	h.say(alice, alice, "/hot")
	h.tap(alice, 1, "📺 豆瓣热门剧集")
	h.tap(alice, 1, "1. 深渊无间 (2026)")
	h.shows(1, "选择要订阅的季")
	h.say(alice, alice, "/hot")
	h.tap(alice, 2, "📺 豆瓣热门剧集")
	h.tap(alice, 2, "下一页 ›")
	h.mp.setRoute(searchPath, ok("search_slow_horses_s6.json"))
	h.tap(alice, 2, "10. 流人 第六季 (2026)")
	h.shows(2, "没有对应的条目？")
	h.tr.verify(t)
}

// The anime calendar shows when each show first airs.
func TestAnimeCalendarShowsAirDates(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{animePath: ok("chart_anime.json")}})
	h.say(alice, alice, "/start")
	h.tap(alice, 1, "🔥 发现")
	h.tap(alice, 1, "🎌 新番放送")
	h.shows(1, "2026-10-05 首播")
	h.tr.verify(t)
}
