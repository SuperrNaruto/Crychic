package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
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
// a paused subscription is marked, and a week with nothing airing says so.
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
	h.mp.setRoute(subsPath, ok(writeFixture(t, airingWithFirstPaused(h))))
	h.mp.setRoute(airingSeasonB, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.say(alice, alice, "/upcoming")
	h.shows(2, "第 1 季 第 15 集 · 已暂停")
	h.shows(2, "有 1 部剧暂时没读到播出时间")
	h.mp.setRoute(subsPath, ok("subscriptions_none.json"))
	h.mp.setRoute(tvHistory, ok("subscribe_history_tv.json"))
	h.tap(alice, 2, "1")
	h.shows(2, "已经结束或被取消")
	h.tap(alice, 2, "订阅历史")
	h.shows(2, "订阅历史 · 电视剧")
	h.say(alice, alice, "/upcoming")
	h.shows(3, "都没有新集播出")
	h.tr.verify(t)
}

// airingWithFirstPaused is subscriptions_airing.json with 择日飞升 paused.
func airingWithFirstPaused(h *harness) []map[string]any {
	h.t.Helper()
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(fixture(h.mp, "subscriptions_airing.json"), &env); err != nil {
		h.t.Fatal(err)
	}
	env.Data[0]["state"] = "S"
	return env.Data
}

// A subscription counting its episodes in a TMDB episode group has its
// season read in that group, as MoviePilot itself does: there 择日飞升's
// season 1 airs episode 3 this Saturday, not the show's own episode 15.
func TestUpcomingReadsEpisodeGroup(t *testing.T) {
	sub := map[string]any{
		"id": 1, "name": "择日飞升", "year": "2026", "type": "电视剧",
		"media_source": "themoviedb", "media_id": "326695", "season": 1,
		"state": "R", "total_episode": 12, "lack_episode": 10, "episode_group": "64a1f0c2",
	}
	grouped := []map[string]any{
		{"air_date": "2026-09-26", "episode_number": 2, "name": "弑神者许应", "season_number": 1},
		{"air_date": "2026-10-10", "episode_number": 3, "name": "元会之争", "season_number": 1},
	}
	routes := upcomingRoutes()
	routes[subsPath] = ok(writeFixture(t, []map[string]any{sub}))
	routes[episodeGroupSeason(airingSeasonA, "64a1f0c2")] = ok(writeFixture(t, grouped))
	h := start(t, scenario{routes: routes})
	h.say(alice, alice, "/upcoming")
	h.shows(1, "第 1 季 第 3 集")
	if strings.Contains(mustMessage(h, 1).text, "第 15 集") {
		t.Error("the calendar read the show's own season, not the subscription's episode group")
	}
	h.tr.verify(t)
}
