package e2e

import (
	"strconv"
	"testing"
)

const (
	noticeChannel int64 = -1001234567890
	channelName         = "@crychic_arrivals"
)

// A configured destination replaces requester delivery. Joining one watch
// from two chats and restarting must still publish once, without identities.
func TestArrivalsGoToChannelOnce(t *testing.T) {
	channelArrival(t, strconv.FormatInt(noticeChannel, 10))
}

// Public channel usernames use Telegram's chat_id contract too.
func TestArrivalsGoToNamedChannel(t *testing.T) {
	channelArrival(t, channelName)
}

func channelArrival(t *testing.T, destination string) {
	t.Helper()
	h := start(t, scenario{notifyChat: destination, routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"),
		duneLookup: ok("subscription_existing.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.shows(1, "通知频道")
	h.say(bob, group, "/search 沙丘")
	h.tap(bob, 2, duneMovie)
	h.restart()
	h.arrives(noticeChannel, duneFile())
	h.shows(3, "在 Emby 中观看")
	h.transfers(duneFile()) // a later copy must not repeat the notice
	h.tr.verify(t)
}
