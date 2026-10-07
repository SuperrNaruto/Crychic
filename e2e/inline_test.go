package e2e

import "testing"

// Typing the bot's name in any chat lists media to share; a shared card's
// button opens that media in a private chat with the bot, ready to
// subscribe. Nobody off the whitelist gets results, and a one-letter query
// is not searched.
func TestInlineSearchAndDeepLink(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:  ok("search_mygo.json"),
		mygoDetails: ok("detail_mygo.json"),
		seasonsPath: ok("seasons_mygo.json"),
		mygoLookup:  ok("subscription_none.json"),
	}})
	h.inline(alice, "迷途之子")
	h.inline(alice, "迷")
	h.inline(stranger, "迷途之子")
	h.say(alice, alice, "/start m_tv_t_224207")
	h.shows(1, "想订哪一季呀")
	h.say(alice, alice, "/start m_xx_t_224207")
	h.shows(2, "Crychic")
	h.tr.verify(t)
}
