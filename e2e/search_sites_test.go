package e2e

import (
	"net/http"
	"strings"
	"testing"
)

// Choosing upstream scope is a metadata read, unlike filtering already found
// resources. A failed site read must not silently broaden or start the search.
func TestSearchSiteSelectionLimitsJSONRequests(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"),
		duneLookup: ok("subscription_none.json"), duneTorrents: searchingSites("torrents_dune.json"),
		duneSites: {status: http.StatusServiceUnavailable, fixture: "server_error.json"},
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, searchSiteMenu)
	h.shows(1, "要订阅《沙丘》")
	h.mp.setRoute(duneSites, ok("empty.json"))
	h.tap(alice, 1, searchSiteMenu)
	h.shows(1, "要订阅《沙丘》")
	h.mp.setRoute(duneSites, ok(searchableSites(t)))
	h.tap(alice, 1, searchSiteMenu)
	h.tap(alice, 1, "2")
	h.shows(1, "站点乙")
	if strings.Contains(h.tr.String(), "-> MoviePilot "+duneTorrents) {
		t.Fatal("opening or selecting search sites already searched the PT sites")
	}
	h.tap(alice, 1, "返回")
	h.shows(1, "要订阅《沙丘》")
	h.tap(alice, 1, searchSiteMenu)
	h.tap(alice, 1, "2")
	h.searchTorrents(alice, 1, "开始搜索")
	if !strings.Contains(h.tr.String(), "mtype=电影&sites=7") {
		t.Fatal("JSON search lost the explicitly chosen site scope")
	}
	h.tap(alice, 1, "取消")
	h.neverShowsSiteCredentials()
	h.tr.verify(t)
}

func TestSearchSiteSelectionUsesCookieStream(t *testing.T) {
	h, s := streamingMovie(t)
	h.mp.setRoute(duneSites, ok(searchableSites(t)))
	s.query = "3,7"
	h.tap(alice, 1, searchSiteMenu)
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "2")
	h.tap(alice, 1, "开始搜索")
	s.allow()
	h.wait(s.entered, "searching the chosen sites with a resource Cookie")
	finishStream(h, s, []int{1})
	batchShows(h, 1, "搜索完成")
	h.tap(alice, 1, "取消")
	h.streamCount(1)
	h.tr.verify(t)
}
