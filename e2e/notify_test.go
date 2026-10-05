package e2e

import "testing"

const dunePoster = "https://image.tmdb.org/t/p/w500/6hsJknqlPceFxExOe87z5VGgNG9.jpg"

// show is a TV show as MoviePilot's transfer history names it.
type show struct{ title, year, id, image string }

var (
	breakingBad = show{"绝命毒师", "2008", "1396", "https://image.tmdb.org/t/p/w500/rqliuvX7NdknSHu5qaSDfESplQi.jpg"}
	conan       = show{"名侦探柯南", "1996", "30983", "https://image.tmdb.org/t/p/w500/7qBrY88hNwMrMb75PkBZjhFolbL.jpg"}
	mygo        = show{"迷途之子!!!!!", "2023", "224207", "https://image.tmdb.org/t/p/w500/dDknWHLYaQB76QSViNdzXOC5v64.jpg"}
)

func duneFile() transfer {
	return transfer{Title: "沙丘", Type: "电影", MediaSource: "themoviedb", MediaID: "438631", Image: dunePoster, Year: "2021"}
}

// file is a transfer record of episodes, e.g. file("S02", "E01-E03").
func (s show) file(season, episodes string) transfer {
	return transfer{Title: s.title, Type: "电视剧", MediaSource: "themoviedb", MediaID: s.id, Seasons: season, Episodes: episodes, Image: s.image, Year: s.year}
}

// The requester hears once their movie is in the library; transfers that
// predate the bot are never announced.
func TestRequesterHearsWhenMovieArrives(t *testing.T) {
	h := start(t, scenario{
		history: []transfer{breakingBad.file("S01", "E01")},
		routes: map[string]route{
			searchPath:    ok("search_dune.json"),
			duneDetails:   ok("detail_dune.json"),
			duneLookup:    ok("subscription_none.json"),
			subscribePath: ok("subscribe_created.json"),
		},
	})
	h.say(alice, alice, "/request 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "入库后会通知你")
	h.arrives(alice, duneFile())
	h.shows(2, "已入库")
	h.tr.verify(t)
}

// In a group the notice mentions the requester, a batch of episodes becomes
// one notice, and other seasons are not announced.
func TestGroupHearsArrivedEpisodesTogether(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_breaking_bad.json"),
		breakDetails:  ok("detail_breaking_bad.json"),
		seasonsPath:   ok("seasons_breaking_bad.json"),
		breakingQuery: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, group, "/request 绝命毒师")
	h.tap(alice, 1, "1. 绝命毒师 (2008)")
	h.tap(alice, 1, "第 2 季 · 13 集")
	h.tap(alice, 1, "从第 1 集开始")
	h.arrives(group,
		breakingBad.file("S02", "E01"),
		breakingBad.file("S01", "E05"),
		breakingBad.file("S02", "E02-E03"),
		breakingBad.file("S02", "E05"),
	)
	h.shows(2, "E01–E03、E05 已入库")
	h.tr.verify(t)
}

// Episodes that arrive one poll apart share a notice once they settle, and
// arrivals not yet announced survive a restart.
func TestEpisodesArrivingApartShareOneNotice(t *testing.T) {
	h := start(t, scenario{quiet: "2s", routes: map[string]route{
		searchPath:    ok("search_breaking_bad.json"),
		breakDetails:  ok("detail_breaking_bad.json"),
		seasonsPath:   ok("seasons_breaking_bad.json"),
		breakingQuery: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/request 绝命毒师")
	h.tap(alice, 1, "1. 绝命毒师 (2008)")
	h.tap(alice, 1, "第 2 季 · 13 集")
	h.tap(alice, 1, "从第 1 集开始")
	h.transfers(breakingBad.file("S02", "E01"))
	h.restart()
	h.arrives(alice, breakingBad.file("S02", "E02"))
	h.shows(2, "E01–E02 已入库")
	h.tr.verify(t)
}

// Asking for something already subscribed still earns a notice.
func TestAlreadySubscribedRequesterIsNotified(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:  ok("search_dune.json"),
		duneDetails: ok("detail_dune.json"),
		duneLookup:  ok("subscription_existing.json"),
	}})
	h.say(alice, alice, "/request 沙丘")
	h.tap(alice, 1, duneMovie)
	h.shows(1, "已在订阅中，入库后会通知你")
	h.arrives(alice, duneFile())
	h.tr.verify(t)
}

// Requests outlive restarts; episodes before the chosen start are ignored,
// and the requester is told when everything they asked for has arrived.
func TestRequestsSurviveRestart(t *testing.T) {
	h := start(t, scenario{routes: conanRoutes()})
	h.say(alice, alice, "/request 名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.tap(alice, 1, conanFirst)
	h.tap(alice, 1, "只追新集（第 1216 集起）")
	h.restart()
	h.arrives(alice,
		conan.file("S01", "E1215"),
		conan.file("S01", "E1216"),
	)
	h.shows(2, "全部入库")
	h.tr.verify(t)
}

// A notice waits until the media server shows what arrived, then links to
// where it can be watched.
func TestNoticeWaitsForTheMediaServer(t *testing.T) {
	h := start(t, scenario{lagging: true, routes: map[string]route{
		searchPath:    ok("search_dune.json"),
		duneDetails:   ok("detail_dune.json"),
		duneLookup:    ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/request 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "确认订阅")
	h.transfers(duneFile())
	h.catchUp(alice)
	h.shows(2, "在 Emby 中观看")
	h.tr.verify(t)
}
