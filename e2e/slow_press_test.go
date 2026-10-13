package e2e

import (
	"strings"
	"testing"
	"time"
)

// slowPressDeadline is short so the scenario need not wait out the
// production default; the held call keeps the flow from returning until the
// test lets it.
const slowPressDeadline = 500 * time.Millisecond

// A press whose flow outlasts the answer deadline is answered at once, so
// the spinner stops; its card still arrives when the flow returns, without
// a second answer.
func TestSlowPressIsAnsweredBeforeItsReply(t *testing.T) {
	h := start(t, scenario{
		answerDeadline: slowPressDeadline,
		routes: map[string]route{
			searchPath:  ok("search_dune.json"),
			duneDetails: ok("detail_dune.json"),
			duneLookup:  ok("subscription_none.json"),
		},
	})
	h.say(alice, alice, "沙丘")
	call := h.mp.holdCalls(duneDetails, 1)[0]
	answered := h.press(alice, 1, "1")
	h.wait(call.entered, "the held card details read")
	h.wait(answered, "the answer to the slow press")
	if strings.Contains(mustMessage(h, 1).text, "要订阅") {
		t.Fatal("the card was shown before its details were read")
	}
	shown := h.tg.expect("edit:1")
	call.allow()
	h.wait(shown, "the card edit")
	h.shows(1, "要订阅《沙丘》")
	// the next press on the message waits for the slow one to finish
	h.tap(alice, 1, "返回")
	if got := strings.Count(h.tr.String(), "<< answerCallbackQuery"); got != 2 {
		t.Errorf("%d answers for two presses", got)
	}
	h.tr.verify(t)
}
