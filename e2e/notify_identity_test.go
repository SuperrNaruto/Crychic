package e2e

import (
	"maps"
	"strings"
	"testing"
)

const reusedSubscriptionID = 1

// pendingReusedSubscriptions leaves Alice waiting for Dune and Bob for
// MyGO, whose new subscription reuses Dune's completed row id. These are
// normal requests; no stale callbacks or internal state edits are needed.
func pendingReusedSubscriptions(t *testing.T, sc scenario) *harness {
	t.Helper()
	const duneRequest, mygoRequest = 1, 2
	mygoSub := map[string]any{
		"id": reusedSubscriptionID, "name": mygo.title, "year": mygo.year,
		"type": "电视剧", "media_source": "themoviedb", "media_id": mygo.id,
		"season": 1, "state": "R", "total_episode": 13, "lack_episode": 13,
	}
	routes := map[string]route{
		searchPath:    ok("search_dune.json"),
		duneDetails:   ok("detail_dune.json"),
		duneLookup:    ok("subscription_none.json"),
		mygoDetails:   ok("detail_mygo.json"),
		seasonsPath:   ok("seasons_mygo.json"),
		mygoLookup:    ok("subscription_none.json"),
		subscribePath: ok(writeFixture(t, map[string]any{"id": reusedSubscriptionID})),
		deleteFirst:   ok("subscribe_deleted.json"),
	}
	maps.Copy(routes, sc.routes)
	sc.routes, sc.quiet, sc.stall = routes, "0s", "0s"
	h := start(t, sc)
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, duneRequest, duneMovie)
	h.tap(alice, duneRequest, "确认订阅")
	h.tr.add(">> MoviePilot completes Dune's subscription; MyGO reuses id=1 while Dune still awaits arrival")
	h.mp.setRoute(searchPath, ok("search_mygo.json"))
	h.say(bob, bob, "/search 迷途之子")
	h.tap(bob, mygoRequest, "1")
	h.tap(bob, mygoRequest, "第 1 季")
	h.tap(bob, mygoRequest, "从第 1 集开始")
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{mygoSub})))
	h.mp.setRoute("GET /api/v1/subscribe/1", ok(writeFixture(t, mygoSub)))
	return h
}

// noticeAt checks the observable recipient and content together: the right
// text delivered to the other requester's chat is still a lost notice.
type arrivalCheck struct {
	message int
	chat    int64
	title   string
}

func noticeAt(h *harness, want arrivalCheck) {
	h.t.Helper()
	msg, ok := h.tg.message(want.message)
	if !ok || msg.chat != want.chat || !strings.Contains(msg.text, want.title) || !strings.Contains(msg.text, "MoviePilot 通知") {
		h.t.Errorf("arrival %d: want %q in chat %d, got chat %d text %q", want.message, want.title, want.chat, msg.chat, msg.text)
	}
}

// Reused ids must not merge targets, recipients or library waits. Both
// requests survive restart; MyGO is visible first while Dune waits for the
// library scan, then each requester hears only about their own title.

// Cancelling the replacement subscription drops only its own watch; the
// older download still arrives for its original requester.

// An unrelated subscription occupying the old id cannot keep an orphaned
// watch alive beyond its 72-hour grace. The replacement stays active and
// still notifies its requester after that grace.
