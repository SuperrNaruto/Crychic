package e2e

import "testing"

const (
	diggerDetails = "GET /api/v1/media/1248832"
	diggerLookup  = "GET /api/v1/subscribe/media/1248832"
	abyssDetails  = "GET /api/v1/media/301489"
	trendingPath  = "GET /api/v1/recommend/tmdb_trending"
	doubanTVPath  = "GET /api/v1/recommend/douban_tv_hot"
	nuwaID        = "390200"
)

// A trending TMDB pick is found again by search and goes straight to its
// card, ready to subscribe; cancelling it returns to the chart.
func TestTrendingPickGoesStraightToSubscribe(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		trendingPath:  ok("chart_trending.json"),
		searchPath:    ok("search_digger.json"),
		diggerDetails: ok("detail_digger.json"),
		diggerLookup:  ok("subscription_none.json"),
	}})
	h.say(alice, alice, "/hot")
	h.tap(alice, 1, "🔥 TMDB 流行趋势")
	h.tap(alice, 1, "1")
	h.shows(1, "确认订阅《挖掘者》")
	h.tap(alice, 1, "取消")
	h.shows(1, "已取消。")
	h.shows(1, "TMDB 流行趋势 · 第 1/2 页")
	h.tr.verify(t)
}

// Douban picks carry Douban ids, so they are matched by title: a sure
// match goes on to its seasons, an unsure one lets the user choose, and
// cancelling returns to the page the pick was on.
func TestDoubanPicksAreMatchedByTitle(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		doubanTVPath: ok("chart_douban_tv.json"),
		searchPath:   ok("search_abyss.json"),
		abyssDetails: ok("detail_abyss.json"),
		seasonsPath:  ok("seasons_abyss.json"),
	}})
	h.say(alice, alice, "/hot")
	h.tap(alice, 1, "📺 豆瓣热门剧集")
	h.tap(alice, 1, "1")
	h.shows(1, "选择要订阅的季")
	h.say(alice, alice, "/hot")
	h.tap(alice, 2, "📺 豆瓣热门剧集")
	h.tap(alice, 2, "下一页 ›")
	h.mp.setRoute(searchPath, ok("search_slow_horses_s6.json"))
	h.tap(alice, 2, "10")
	h.shows(2, "没有对应的条目？")
	h.tap(alice, 2, "取消")
	h.shows(2, "豆瓣热门剧集 · 第 2/")
	h.tr.verify(t)
}

// The anime calendar opens on today's weekday, read from Bangumi itself,
// and switches to any other; it groups a day's picks by first air date and
// links each to its TMDB page with a collapsed synopsis, found by its title or else its original
// title; a show TMDB lacks, or has no synopsis for, gets Bangumi's, read
// from Bangumi itself. A synopsis Bangumi failed to give is fetched again
// when the page shows again.
func TestAnimeCalendarShowsEachWeekday(t *testing.T) {
	h := start(t, scenario{
		routes: map[string]route{
			searchPath:  ok("empty.json"),
			seasonsPath: ok("seasons_mygo.json"),
		},
		searches: map[string]string{
			"列女战纪：女娲石记":            "search_calendar_nuwa.json",
			"魔法少女育成计划 restart":     "search_calendar_mahou.json",
			"罗梅莉亚战记":               "search_calendar_romelia.json",
			"PSYREN -决战游戏-":        "search_calendar_psyren.json",
			"PSYREN -サイレン-":        "search_calendar_psyren_original.json",
			"你好，我是受心上人所托来做恋爱药的魔女。": "search_calendar_majo.json",
		},
	})
	h.bgm.setDown(nuwaID, true)
	h.say(alice, alice, "/start")
	h.tap(alice, 1, "🔥 发现")
	h.tap(alice, 1, "🎌 新番放送")
	h.shows(1, "星期一（今天）")
	h.shows(1, "2026-10-05 首播")
	h.shows(1, "themoviedb.org/tv/297903")
	h.bgm.setDown(nuwaID, false)
	h.tap(alice, 1, "2")
	h.tap(alice, 1, "取消")
	h.shows(1, "补天的时代已经过去")
	h.shows(1, "bgm.tv/subject/390200")
	h.tap(alice, 1, "二")
	h.shows(1, "星期二 · 第 1/1 页")
	h.shows(1, "2026-10-13 首播")
	h.tap(alice, 1, "三")
	h.shows(1, "这天没有新番放送")
	h.tr.verify(t)
}
