package e2e

import (
	"strings"
	"testing"
)

// 洗版订阅 on a movie the library already holds subscribes it as an upgrade,
// never through a subscription that exists already, and every better
// version arriving afterwards is told, unlike an ordinary movie's.
func TestHeldMovieSubscribesAsUpgrade(t *testing.T) {
	const firstVersion, betterVersion = 2, 3
	h := start(t, scenario{quiet: "0s", routes: map[string]route{
		searchPath:       ok("search_dune.json"),
		duneDetails:      ok("detail_dune.json"),
		libraryMoviePath: ok("library_movie_held.json"),
		duneLookup:       ok("subscription_existing.json"),
		subscribePath:    ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.shows(1, "已经在媒体库里啦")
	h.tap(alice, 1, "洗版订阅")
	if !strings.Contains(h.tr.String(), `notice="ℹ️ 这个已经订阅上啦`) {
		t.Error("an upgrade was offered over an existing subscription")
	}
	h.mp.setRoute(duneLookup, ok("subscription_none.json"))
	h.tap(alice, 1, "洗版订阅")
	h.shows(1, "要洗版订阅《沙丘》吗")
	h.tap(alice, 1, "确认洗版")
	h.shows(1, "帮你订好《沙丘》的洗版啦")
	if !strings.Contains(h.tr.String(), `"best_version":1`) {
		t.Error("the upgrade subscription was written without best_version")
	}
	h.arrives(alice, duneFile())
	noticeAt(h, arrivalCheck{message: firstVersion, chat: alice, title: "沙丘"})
	h.arrives(alice, duneFile())
	noticeAt(h, arrivalCheck{message: betterVersion, chat: alice, title: "沙丘"})
	h.tr.verify(t)
}
