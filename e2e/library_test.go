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
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.shows(1, "《沙丘》已经在媒体库里啦，直接去看吧")
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
	h.say(alice, alice, "/search 碧蓝之海")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 3 季")
	h.shows(1, "《碧蓝之海》第 3 季已经全部在媒体库里啦")
	h.tr.verify(t)
}

// A retained second version of E01 is not another episode: E13 is still
// missing, so the season stays selectable and is complete only on its arrival.
func TestPartlyHeldSeasonCompletesWithLibrary(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:      ok("search_mygo.json"),
		mygoDetails:     ok("detail_mygo.json"),
		seasonsPath:     ok("seasons_mygo.json"),
		libraryShowPath: ok("library_mygo_versions.json"),
		mygoLookup:      ok("subscription_none.json"),
		subscribePath:   ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "多选季…")
	h.tap(alice, 1, "第 1 季")
	h.shows(1, "13 集 · 已有 12 集")
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "第 1 季")
	h.shows(1, "媒体库已有 E01–E12")
	h.tap(alice, 1, "从第 1 集开始")
	h.arrives(alice, mygo.file("S01", "E13"))
	h.shows(2, "这一季你要的剧集全部到齐啦")
	h.tr.verify(t)
}

// Someone asking for what is already coming sees how far the download got.
func TestDownloadInProgressIsShown(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:      ok("search_mygo.json"),
		mygoDetails:     ok("detail_mygo.json"),
		seasonsPath:     ok("seasons_mygo.json"),
		libraryShowPath: ok("library_mygo.json"),
		mygoLookup:      ok("subscription_existing.json"),
		downloadsPath:   ok("downloads_mygo.json"),
	}})
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 1 季")
	h.shows(1, "⬇️ 正在努力下载 E10–E12 · 0%")
	h.shows(1, "早就订阅上啦")
	h.tr.verify(t)
}
