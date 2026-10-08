package e2e

import (
	"strings"
	"testing"
)

// Correcting a failed title keeps download intent, but an explicit 搜索
// prefix overrides it. Neither path starts a download without confirmation.
func TestDownloadIntentStaysWithCorrectedTitle(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok("empty.json"), duneDetails: ok("detail_dune.json"),
		duneLookup: ok("subscription_none.json"), duneTorrents: searchingSites("torrents_dune.json"),
	}})
	h.say(alice, alice, "下载 沙邱")
	h.answer(alice, 1, "沙球")
	h.shows(1, "换个名字")
	h.mp.setRoute(searchPath, ok("search_dune.json"))
	h.answer(alice, 1, "沙丘")
	h.searchTorrents(alice, 1, duneMovie)
	h.shows(1, "《沙丘》的资源")
	h.tap(alice, 1, "取消")
	h.mp.setRoute(searchPath, ok("empty.json"))
	h.say(alice, alice, "下载 沙邱")
	h.mp.setRoute(searchPath, ok("search_dune.json"))
	h.answer(alice, 2, "搜索 沙丘")
	h.tap(alice, 2, duneMovie)
	h.shows(2, "要订阅《沙丘》")
	if strings.Count(h.tr.String(), duneTorrents) != 1 || strings.Contains(h.tr.String(), downloadPath) {
		t.Fatal("correcting a title ignored its explicit intent or started an unconfirmed download")
	}
	h.tr.verify(t)
}

// Related and series picks leave the original title's download intent
// behind. Returning all the way to that title restores its own intent.
func TestDownloadIntentStopsAtRelatedMedia(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"),
		duneLookup: ok("subscription_none.json"), duneTorrents: searchingSites("torrents_dune.json"),
		duneRecommend: ok("recommend_dune.json"), dunePartTwoDetails: ok("detail_dune_part_two.json"),
		dunePartTwoLookup: ok("subscription_none.json"), duneCollection: ok("collection_dune.json"),
		collectionSearch("沙丘"): ok("collections_dune.json"),
	}})
	h.say(alice, alice, "下载 沙丘")
	h.searchTorrents(alice, 1, duneMovie)
	h.tap(alice, 1, "返回")
	for _, choice := range []struct{ browse, pick string }{{"相似推荐", "1"}, {"同系列", "2"}} {
		h.tap(alice, 1, choice.browse)
		h.tap(alice, 1, choice.pick)
		h.shows(1, "要订阅")
		if _, offered := findButton(mustMessage(h, 1).rows, searchResources); !offered {
			t.Fatal("a related pick must let the user request its resource search explicitly")
		}
		h.tap(alice, 1, "返回")
		h.tap(alice, 1, "返回")
	}
	if strings.Count(h.tr.String(), "GET /api/v1/search/media/") != 1 {
		t.Fatal("browsing related media searched its resources without being asked")
	}
	h.tap(alice, 1, "返回")
	h.searchTorrents(alice, 1, duneMovie)
	h.shows(1, "《沙丘》的资源")
	h.tr.verify(t)
}
