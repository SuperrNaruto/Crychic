package e2e

import "testing"

const (
	grandBlueDetails = "GET /api/v1/media/79166"
	mygoDetails      = "GET /api/v1/media/224207"
	mygoLookup       = "GET /api/v1/subscribe/media/224207"
)

// A movie already in the library is watchable now, not subscribed again.
func TestHeldMovieIsNotSubscribedAgain(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:       ok("search_dune.json"),
		duneDetails:      ok("detail_dune.json"),
		libraryMoviePath: ok("library_movie_held.json"),
	}})
	h.say(alice, alice, "/request 沙丘")
	h.tap(alice, 1, duneMovie)
	h.shows(1, "《沙丘》已在媒体库中，可以直接观看")
	h.tr.verify(t)
}

// Season buttons show what the library holds; a season held in full ends the
// request instead of subscribing it again.
func TestHeldSeasonIsNotSubscribedAgain(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:       ok("search_grand_blue.json"),
		grandBlueDetails: ok("detail_grand_blue.json"),
		seasonsPath:      ok("seasons_grand_blue.json"),
		libraryShowPath:  ok("library_grand_blue.json"),
	}})
	h.say(alice, alice, "/request 碧蓝之海")
	h.tap(alice, 1, "1. 碧蓝之海 (2018)")
	h.tap(alice, 1, "第 3 季 · 12 集 · 已入库")
	h.shows(1, "《碧蓝之海》第 3 季已全部在媒体库中")
	h.tr.verify(t)
}

// A season partly in the library names what is there, and its requester is
// told the season is complete once the missing episodes arrive.
func TestPartlyHeldSeasonCompletesWithLibrary(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:      ok("search_mygo.json"),
		mygoDetails:     ok("detail_mygo.json"),
		seasonsPath:     ok("seasons_mygo.json"),
		libraryShowPath: ok("library_mygo.json"),
		mygoLookup:      ok("subscription_none.json"),
		subscribePath:   ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/request 迷途之子")
	h.tap(alice, 1, "1. 迷途之子!!!!! (2023)")
	h.tap(alice, 1, "第 1 季 · 13 集 · 已有 1 集")
	h.shows(1, "媒体库已有 E13")
	h.tap(alice, 1, "从第 1 集开始")
	h.arrives(alice, episodeFile("迷途之子!!!!!", "224207", "S01", "E01-E12", mygoImage))
	h.shows(2, "本季请求的剧集已全部入库")
	h.tr.verify(t)
}
