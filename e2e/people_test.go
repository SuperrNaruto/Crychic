package e2e

import "testing"

const (
	castLabel      = "演职人员"
	duneCredits    = "GET /api/v1/tmdb/credits/438631/电影"
	chalametWorks  = "GET /api/v1/tmdb/person/credits/1190668"
	mygoBangumi    = "GET /api/v1/bangumi/credits/428735"
	mygoDetails428 = "GET /api/v1/media/428735"
	saeWorks       = "GET /api/v1/bangumi/person/credits/24387"
	chalametDune2  = "4" // 沙丘2 in the recorded works
	saeStarKaidou  = "1" // スター街道, listed once though credited three times
	saeBangumiName = "大塚紗英"
)

// From a movie's card, someone lists its cast, opens an actor's works and
// picks one: a TMDB work is found by its identity and opens its own card;
// 返回 leads back through the works.
func TestBrowseCastAndWorks(t *testing.T) {
	h := start(t, scenario{
		routes: map[string]route{
			searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"), duneLookup: ok("subscription_none.json"),
			duneCredits: ok("credits_dune.json"), chalametWorks: ok("person_works_tmdb.json"),
			dunePartTwoDetails: ok("detail_dune_part_two.json"), dunePartTwoLookup: ok("subscription_none.json"),
		},
		searches: map[string]string{"沙丘2": "search_dune.json"},
	})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, castLabel)
	h.shows(1, "Paul Atreides")
	h.tap(alice, 1, "1")
	h.shows(1, "提莫西·查拉梅 的作品")
	h.tap(alice, 1, chalametDune2)
	h.shows(1, "沙丘2")
	h.tap(alice, 1, "返回")
	h.shows(1, "提莫西·查拉梅 的作品")
	h.tr.verify(t)
}

// Bangumi credits voice actors with the characters they play. A work
// credited for several roles is listed once, and one MoviePilot cannot find
// to subscribe says so instead of guessing.
func TestVoiceActorWorksFromBangumi(t *testing.T) {
	h := start(t, scenario{searches: map[string]string{"スター街道": "empty.json"}, routes: map[string]route{
		searchPath: ok("search_mygo_bangumi.json"), mygoDetails428: ok("detail_mygo_bangumi.json"),
		seasonsPath: ok("seasons_mygo_bangumi.json"), mygoBangumi: ok("credits_mygo_bangumi.json"),
		"GET /api/v1/subscribe/media/428735": ok("subscription_none.json"),
		saeWorks:                             ok("person_works_bangumi.json"),
	}})
	h.say(alice, alice, "/search MyGO")
	h.tap(alice, 1, castLabel)
	h.shows(1, "花園たえ")
	h.tap(alice, 1, "4")
	h.shows(1, saeBangumiName+" 的作品")
	h.tap(alice, 1, saeStarKaidou)
	h.shows(1, "没找到「スター街道」能订阅的条目")
	h.tr.verify(t)
}
