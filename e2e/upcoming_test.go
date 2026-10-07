package e2e

import (
	"net/http"
	"testing"
)

const (
	upcomingFeature = "追剧日历"
	airingSeasonA   = "GET /api/v1/tmdb/326695/1"
	airingSeasonB   = "GET /api/v1/tmdb/283938/1"
	airingSeasonC   = "GET /api/v1/tmdb/321518/1"
)

func upcomingRoutes() map[string]route {
	return map[string]route{
		subsPath:      ok("subscriptions_airing.json"),
		airingSeasonA: ok("tmdb_season_326695_1.json"),
		airingSeasonB: ok("tmdb_season_283938_1.json"),
		airingSeasonC: ok("tmdb_season_321518_1.json"),
	}
}

// The calendar lists the subscribed seasons' episodes airing this week,
// one day after another from today; a number opens the subscription and
// 返回 shows the calendar again. A season that cannot be read is named,
// and a week with nothing airing says so.
func TestUpcomingEpisodes(t *testing.T) {
	h := start(t, scenario{routes: upcomingRoutes()})
	h.say(alice, alice, "/start")
	h.tap(alice, 1, upcomingFeature)
	h.shows(1, "10月7日 周三")
	h.shows(1, "试着使用了互联网！")
	h.shows(1, "第 1 季 第 15 集")
	h.tap(alice, 1, "2")
	h.shows(1, "订阅详情")
	h.tap(alice, 1, "返回")
	h.shows(1, "追剧日历")
	h.mp.setRoute(airingSeasonB, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.say(alice, alice, "/upcoming")
	h.shows(2, "有 1 部剧暂时没读到播出时间")
	h.mp.setRoute(subsPath, ok("subscriptions_none.json"))
	h.say(alice, alice, "/upcoming")
	h.shows(3, "都没有新集播出")
	h.tr.verify(t)
}
