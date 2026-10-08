package e2e

import (
	"net/http"
	"testing"
	"time"
)

// An unquoted private title after an episode prompt expires starts its
// search immediately. An explicit answer to the expired prompt does not.
func TestExpiredPrivateInputReleasesThePrompt(t *testing.T) {
	const beyondSessionLifetime = 11 * time.Minute
	h := start(t, scenario{routes: conanRoutes(), searches: map[string]string{
		"名侦探柯南": "search_conan.json",
		"沙丘":    "search_dune.json",
	}})
	h.say(alice, alice, "名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.tap(alice, 1, conanFirst)
	h.tap(alice, 1, "指定起始集…")
	h.advance(beyondSessionLifetime)
	h.say(alice, alice, "沙丘")
	h.shows(2, "这些「沙丘」啦")
	h.say(alice, alice, "名侦探柯南")
	h.tap(alice, 3, conanShow)
	h.tap(alice, 3, conanFirst)
	h.tap(alice, 3, "指定起始集…")
	h.advance(beyondSessionLifetime)
	h.answerQuoting(alice, 3, "500")
	h.shows(3, "过期啦")
	h.say(alice, alice, "沙丘")
	h.shows(4, "这些「沙丘」啦")
	h.tr.verify(t)
}

// Read failures keep the chosen media and season. Each retry resumes the
// failed lookup instead of repeating the search and card enrichment.
func TestReadRetriesKeepMediaAndSeason(t *testing.T) {
	routes := conanRoutes()
	routes[seasonsPath] = route{status: http.StatusServiceUnavailable, fixture: "server_error.json"}
	h := start(t, scenario{routes: routes})
	h.say(alice, alice, "名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.shows(1, "MoviePilot 好像打了个盹")
	h.tap(alice, 1, "返回")
	h.shows(1, "这些「名侦探柯南」啦")
	h.tap(alice, 1, conanShow)
	h.mp.setRoute(seasonsPath, ok("seasons_conan.json"))
	h.tap(alice, 1, "重试")
	h.mp.setRoute(conanLookup, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, 1, conanFirst)
	h.shows(1, "MoviePilot 好像打了个盹")
	h.mp.setRoute(conanLookup, ok("subscription_none.json"))
	h.tap(alice, 1, "重试")
	h.shows(1, "要订阅《名侦探柯南》第 1 季")
	h.tap(alice, 1, "从第 1 集开始")
	h.shows(1, "帮你订好《名侦探柯南》第 1 季")
	h.tr.verify(t)
}

// Shortcuts do not strand an initial reply on failure, nor add hidden
// result/season pages to the back stack when a retry succeeds.
func TestSingleResultReadRetryKeepsConfirmation(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:   ok("search_abyss.json"),
		abyssDetails: ok("detail_abyss.json"),
		seasonsPath:  {status: http.StatusServiceUnavailable, fixture: "server_error.json"},
		abyssLookup:  {status: http.StatusServiceUnavailable, fixture: "server_error.json"},
	}})
	h.say(alice, alice, "深渊无间")
	h.mp.setRoute(seasonsPath, ok("seasons_abyss.json"))
	h.tap(alice, 1, "重试")
	h.shows(1, "MoviePilot 好像打了个盹")
	h.mp.setRoute(abyssLookup, ok("subscription_none.json"))
	h.tap(alice, 1, "重试")
	h.shows(1, "要订阅《深渊无间》第 1 季")
	if _, back := findButton(mustMessage(h, 1).rows, "返回"); back {
		t.Fatal("shortcuts must not leave hidden pages in the back stack")
	}
	h.tap(alice, 1, "重新搜索")
	h.tap(alice, 1, "返回")
	h.shows(1, "要订阅《深渊无间》第 1 季")
	h.tr.verify(t)
}

// A failed cosmetic library read is unknown, not evidence of absence.
// The owner can still explicitly confirm the request.
func TestUnavailableLibraryDoesNotBlockConfirmedRequest(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:       ok("search_dune.json"),
		duneDetails:      ok("detail_dune.json"),
		duneLookup:       ok("subscription_none.json"),
		libraryMoviePath: {status: http.StatusServiceUnavailable, fixture: "server_error.json"},
		subscribePath:    ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "沙丘")
	h.tap(alice, 1, duneMovie)
	h.shows(1, "暂时看不到媒体库里有没有它")
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "帮你订好《沙丘》")
	h.tr.verify(t)
}

// A write may have succeeded despite a server error or missing receipt.
// Neither outcome is retried or advertised as a confirmed subscription.
func TestUncertainSubscriptionIsNotRetried(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_dune.json"),
		duneDetails:   ok("detail_dune.json"),
		duneLookup:    ok("subscription_none.json"),
		subscribePath: {status: http.StatusServiceUnavailable, fixture: "server_error.json"},
	}})
	h.say(alice, alice, "沙丘")
	h.tap(alice, 1, duneMovie)
	old, _ := findButton(mustMessage(h, 1).rows, "确认订阅")
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "没能确认订阅有没有成功")
	h.shows(1, "/subscribe")
	h.tapData(alice, 1, old)
	h.shows(1, "没能确认订阅有没有成功")
	h.mp.setRoute(subscribePath, ok("subscription_none.json"))
	h.say(alice, alice, "沙丘")
	h.tap(alice, 2, duneMovie)
	h.tap(alice, 2, "确认订阅")
	h.shows(2, "没能确认订阅有没有成功")
	h.transfers(duneFile())
	h.tr.verify(t)
}
