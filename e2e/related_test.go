package e2e

import "testing"

const (
	dunePartTwoDetails = "GET /api/v1/media/693134"
	dunePartTwoLookup  = "GET /api/v1/subscribe/media/693134"
	duneRecommend      = "GET /api/v1/tmdb/recommend/438631/电影"
	duneCollection     = "GET /api/v1/tmdb/collection/726871"
)

// From a picked movie, someone browses its recommendations, picks the
// sequel, opens the sequel's series (found by the stem of its title, 沙丘2 →
// 沙丘) and subscribes to the first film instead.
func TestBrowseRecommendationsAndSeries(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:             ok("search_dune.json"),
		duneDetails:            ok("detail_dune.json"),
		duneLookup:             ok("subscription_none.json"),
		duneRecommend:          ok("recommend_dune.json"),
		dunePartTwoDetails:     ok("detail_dune_part_two.json"),
		dunePartTwoLookup:      ok("subscription_none.json"),
		collectionSearch("沙丘"): ok("collections_dune.json"),
		duneCollection:         ok("collection_dune.json"),
		subscribePath:          ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/request 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "相似推荐")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "同系列")
	h.shows(1, "「沙丘2」所属系列")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "已订阅《沙丘》")
	h.tr.verify(t)
}

// A movie in no series says so and keeps its card, still subscribable.
func TestMovieWithoutSeriesKeepsItsCard(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:                           ok("search_dune.json"),
		"GET /api/v1/subscribe/media/911972": ok("subscription_none.json"),
		subscribePath:                        ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/request 沙丘")
	h.tap(alice, 1, "8")
	h.tap(alice, 1, "同系列")
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "已订阅《沙丘虫暴》")
	h.tr.verify(t)
}
