package e2e

import (
	"strings"
	"testing"
	"time"
)

const (
	diggerDetails = "GET /api/v1/media/1248832"
	diggerLookup  = "GET /api/v1/subscribe/media/1248832"
	abyssDetails  = "GET /api/v1/media/301489"
	trendingPath  = "GET /api/v1/recommend/tmdb_trending"
	doubanTVPath  = "GET /api/v1/recommend/douban_tv_hot"
	nuwaID        = "390200"
)

// A trending TMDB pick is found again by search and goes straight to its
// card, ready to subscribe; 返回 leads back to the chart.
func TestTrendingPickGoesStraightToSubscribe(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		trendingPath:  ok("chart_trending.json"),
		searchPath:    ok("search_digger.json"),
		diggerDetails: ok("detail_digger.json"),
		diggerLookup:  ok("subscription_none.json"),
	}})
	h.say(alice, alice, "/hot")
	h.tap(alice, 1, "TMDB 流行趋势")
	h.tap(alice, 1, "1")
	h.shows(1, "确认订阅《挖掘者》")
	h.tap(alice, 1, "返回")
	h.shows(1, "TMDB 流行趋势 · 第 1/2 页")
	h.tr.verify(t)
}

// Two taps while a synopsis retries must not share writable session slices
// or let the earlier result overwrite a later navigation choice.
func TestCalendarRetryOverlappingNavigation(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{searchPath: ok("empty.json")}})
	h.bgm.setDown(nuwaID, true)
	h.say(alice, alice, "/hot")
	h.tap(alice, 1, "新番放送")
	const slowLookup = 100 * time.Millisecond
	entered := h.bgm.delay(nuwaID, slowLookup)
	first := h.press(alice, 1, "·一·")
	h.wait(entered, "the first synopsis retry")
	h.waitLookup("title=列女战纪：女娲石记")
	second := h.press(alice, 1, "·一·")
	h.wait(first, "the first calendar tap")
	h.wait(second, "the second calendar tap")
	h.tap(alice, 1, "二")
	h.shows(1, "星期二")
	h.tr.verify(t)
}

// Observe both reads before the overlapping tap, rather than imposing an
// arbitrary order between that tap and an independent backend lookup.
func (h *harness) waitLookup(want string) {
	h.t.Helper()
	const checkEvery = time.Millisecond
	const initialAndRetry = 2
	deadline := time.After(actionTimeout)
	tick := time.NewTicker(checkEvery)
	defer tick.Stop()
	for {
		if strings.Count(h.tr.String(), want) >= initialAndRetry {
			return
		}
		select {
		case <-tick.C:
		case <-deadline:
			h.t.Fatal("calendar retry search did not arrive")
		}
	}
}

// Douban picks carry Douban ids, so they are matched by title: a sure
// match goes on to its seasons, an unsure one lets the user choose, and
// 返回 leads back to the page the pick was on.
func TestDoubanPicksAreMatchedByTitle(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		doubanTVPath: ok("chart_douban_tv.json"),
		searchPath:   ok("search_abyss.json"),
		abyssDetails: ok("detail_abyss.json"),
		seasonsPath:  ok("seasons_abyss.json"),
		abyssLookup:  ok("subscription_none.json"),
	}})
	h.say(alice, alice, "/hot")
	h.tap(alice, 1, "豆瓣热门剧集")
	h.tap(alice, 1, "1")
	h.shows(1, "确认订阅《深渊无间》第 1 季")
	h.say(alice, alice, "/hot")
	h.tap(alice, 2, "豆瓣热门剧集")
	h.tap(alice, 2, "下一页 ›")
	h.mp.setRoute(searchPath, ok("search_slow_horses_s6.json"))
	h.tap(alice, 2, "10")
	h.shows(2, "没有对应的条目？")
	h.tap(alice, 2, "返回")
	h.shows(2, "豆瓣热门剧集 · 第 2/")
	h.mp.setRoute(searchPath, ok("search_calendar_psyren.json"))
	h.tap(alice, 2, "10")
	h.shows(2, "没有对应的条目？")
	h.shows(2, `<li value="1">`)
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
	h.tap(alice, 1, "发现")
	h.tap(alice, 1, "新番放送")
	h.shows(1, "星期一（今天）")
	h.shows(1, "2026-10-05 首播")
	h.shows(1, "themoviedb.org/tv/297903")
	h.bgm.setDown(nuwaID, false)
	h.tap(alice, 1, "2")
	h.tap(alice, 1, "返回")
	h.shows(1, "补天的时代已经过去")
	h.shows(1, "bgm.tv/subject/390200")
	h.tap(alice, 1, "二")
	h.shows(1, "星期二 · 第 1/1 页")
	h.shows(1, "2026-10-13 首播")
	h.tap(alice, 1, "三")
	h.shows(1, "这天没有新番放送")
	h.tr.verify(t)
}
