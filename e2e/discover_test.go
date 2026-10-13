package e2e

import (
	"net/http"
	"strings"
	"testing"
)

const (
	diggerDetails = "GET /api/v1/media/1248832"
	diggerLookup  = "GET /api/v1/subscribe/media/1248832"
	abyssDetails  = "GET /api/v1/media/301489"
	trendingPath  = "GET /api/v1/recommend/tmdb_trending"
	doubanTVPath  = "GET /api/v1/recommend/douban_tv_hot"
	nuwaID        = "390200"
	aonohako      = "207347"
	ashi          = "126437"
)

// A trending TMDB pick is found again by search and goes straight to its
// card, ready to subscribe; 返回 leads back to the chart, and so does the
// subscription's success, so browsing goes on without opening it again.
func TestTrendingPickGoesStraightToSubscribe(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		trendingPath:  ok("chart_trending.json"),
		searchPath:    ok("search_digger.json"),
		diggerDetails: ok("detail_digger.json"),
		diggerLookup:  ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "TMDB 流行趋势")
	h.mp.setRoute(searchPath, ok("empty.json"))
	h.tap(alice, 1, "1")
	h.shows(1, "没找到")
	h.tap(alice, 1, "返回")
	h.shows(1, "第 1/2 页")
	h.mp.setRoute(searchPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, 1, "1")
	h.shows(1, "MoviePilot 好像打了个盹")
	h.tap(alice, 1, "返回")
	h.shows(1, "第 1/2 页")
	h.mp.setRoute(searchPath, ok("search_digger.json"))
	pick, _ := findButton(mustMessage(h, 1).rows, "1")
	h.tap(alice, 1, "1")
	h.tapData(alice, 1, pick)
	h.shows(1, "要订阅《挖掘者》")
	h.tap(alice, 1, "返回")
	h.shows(1, "第 1/2 页")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "帮你订好《挖掘者》")
	h.tap(alice, 1, "返回")
	h.shows(1, "第 1/2 页")
	h.tr.verify(t)
}

// A calendar sequel airs years after its show began, and TMDB dates a show
// by its first season; the sequel is still that TMDB show, so its card has
// TMDB's synopsis and its subscription is one transfers are matched with.
// It opens on the season its name names (青之芦苇 第二季), else on the
// show's only one, where TMDB numbers 青之箱's sequel on from episode 26.
func TestCalendarSequelIsItsTMDBShow(t *testing.T) {
	h := start(t, scenario{
		routes: map[string]route{
			searchPath:                      ok("empty.json"),
			seasonsPath:                     ok("seasons_aonohako.json"),
			"GET /api/v1/media/" + aonohako: ok("detail_aonohako.json"),
			"GET /api/v1/subscribe/media/" + aonohako: ok("subscription_none.json"),
			"GET /api/v1/media/" + ashi:               ok("detail_ashi.json"),
			"GET /api/v1/subscribe/media/" + ashi:     ok("subscription_none.json"),
			subscribePath:                             ok("subscribe_created.json"),
		},
		searches: map[string]string{
			"青之箱 第二季":  "search_calendar_aonohako.json",
			"青之芦苇 第二季": "search_calendar_ashi.json",
		},
	})
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "新番放送")
	h.tap(alice, 1, "日")
	h.shows(1, "themoviedb.org/tv/"+aonohako)
	h.shows(1, "themoviedb.org/tv/"+ashi)
	h.tap(alice, 1, "2")
	h.shows(1, "每天早晨在晨练时")
	h.tap(alice, 1, "只追新集（第 27 集起）")
	h.shows(1, "帮你订好《青之箱》第 1 季（从第 27 集开始）")
	h.tap(alice, 1, "返回")
	h.mp.setRoute(seasonsPath, ok("seasons_ashi.json"))
	h.tap(alice, 1, "3")
	h.shows(1, "要订阅《青之芦苇》第 2 季")
	h.tr.verify(t)
}

// Taps while a page's synopses are still being looked up join that lookup
// rather than repeating it, and must not share writable session slices or
// let an earlier result overwrite a later page; the last page fills in when
// the lookup ends.
func TestCalendarRetryOverlappingNavigation(t *testing.T) {
	h := start(t, scenario{
		noteGrace: mondayNoteGrace,
		progress:  followIdle,
		routes: map[string]route{
			searchPath:               ok("empty.json"),
			bangumiInfoPath + nuwaID: ok(writeFixture(t, map[string]any{"overview": "MoviePilot 返回的女娲简介"})),
		},
	})
	slow := h.mp.holdCalls(bangumiInfoPath+nuwaID, 1)[0]
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "新番放送")
	h.wait(slow.entered, "the slow synopsis lookup")
	first := h.press(alice, 1, "·一·")
	second := h.press(alice, 1, "·一·")
	h.wait(first, "the first calendar tap")
	h.wait(second, "the second calendar tap")
	h.shows(1, "简介补充中")
	if got := strings.Count(h.tr.String(), "title=列女战纪：女娲石记"); got != 1 {
		t.Errorf("the slow pick was searched %d times, want once", got)
	}
	filled := h.tg.expect("edit:1")
	slow.allow()
	h.wait(filled, "the page filling in")
	h.shows(1, "MoviePilot 返回的女娲简介")
	h.tr.verify(t)
}

// missingDoubanPoster is a chart poster Douban's image host no longer has.
const missingDoubanPoster = "/view/photo/m_ratio_poster/public/p2933527614.webp"

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
	h.img.setMissing(missingDoubanPoster)
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "豆瓣热门剧集")
	h.tap(alice, 1, "1")
	h.shows(1, "要订阅《深渊无间》第 1 季")
	h.say(alice, alice, "/trending")
	h.tap(alice, 2, "豆瓣热门剧集")
	h.tap(alice, 2, "下一页 ›")
	h.mp.setRoute(searchPath, ok("search_slow_horses_s6.json"))
	h.tap(alice, 2, "10")
	h.shows(2, "没有你想要的那部？")
	h.tap(alice, 2, "返回")
	h.shows(2, "第 2/2 页")
	h.mp.setRoute(searchPath, ok("search_calendar_psyren.json"))
	h.tap(alice, 2, "10")
	h.shows(2, "没有你想要的那部？")
	h.shows(2, `<li value="1">`)
	h.tr.verify(t)
}

// The anime calendar opens on today's weekday, read from Bangumi itself,
// and switches to any other; it groups a day's picks by first air date and
// links each to its TMDB page with a collapsed synopsis, found by its title or else its original
// title; a show TMDB lacks, or has no synopsis for, gets Bangumi's, read
// from Bangumi itself.
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
	h.say(alice, alice, "/start")
	h.tap(alice, 1, "发现")
	h.tap(alice, 1, "新番放送")
	h.shows(1, "星期一（今天）")
	h.shows(1, "2026-10-05 首播")
	h.shows(1, "themoviedb.org/tv/297903")
	h.tap(alice, 1, "2")
	h.tap(alice, 1, "返回")
	h.shows(1, "补天的时代已经过去")
	h.shows(1, "bgm.tv/subject/390200")
	h.tap(alice, 1, "二")
	h.shows(1, "星期二</h3>")
	h.shows(1, "2026-10-13 首播")
	h.tap(alice, 1, "三")
	h.shows(1, "这天没有新番放送")
	h.tr.verify(t)
}

// A calendar pick whose note has not found its TMDB show yet is still that
// show when Douban lists a show of the same title, year and kind: the card
// opens at once instead of a list asking which one was meant.
func TestCalendarPickIgnoresDoubanTwin(t *testing.T) {
	const romelia = "305313"
	h := start(t, scenario{routes: map[string]route{
		searchPath:                               {status: http.StatusServiceUnavailable, fixture: "server_error.json"},
		seasonsPath:                              ok("seasons_mygo.json"),
		"GET /api/v1/media/" + romelia:           ok("detail_ashi.json"),
		"GET /api/v1/subscribe/media/" + romelia: ok("subscription_none.json"),
	}})
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "新番放送")
	h.mp.setRoute(searchPath, ok("search_calendar_romelia_douban.json"))
	h.tap(alice, 1, "3")
	h.shows(1, "想订哪一季呀")
	h.tr.verify(t)
}
