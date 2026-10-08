package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

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
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "入库了我第一时间叫你")
	h.arrives(alice, duneFile())
	h.shows(2, "入库啦")
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
	h.say(alice, group, "/search 绝命毒师")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 2 季")
	h.tap(alice, 1, "从第 1 集开始")
	h.arrives(group,
		breakingBad.file("S02", "E01"),
		breakingBad.file("S01", "E05"),
		breakingBad.file("S02", "E02-E03"),
		breakingBad.file("S02", "E05"),
	)
	h.shows(2, "E01–E03、E05 到家啦")
	h.tr.verify(t)
}

// Episodes that arrive one poll apart share a notice once they settle, and
// arrivals not yet announced survive a restart. The notice words each
// download's quality once, as MoviePilot's recognizer does from one of its
// file names; a download it cannot recognize adds nothing.
func TestEpisodesArrivingApartShareOneNotice(t *testing.T) {
	const pack, other = "rovers-pack-hash", "unknown-release-hash"
	release := func(episode, hash, name string) transfer {
		f := breakingBad.file("S02", episode)
		f.Src, f.Hash = "/downloads/Breaking.Bad.S02/"+name, hash
		return f
	}
	h := start(t, scenario{quiet: "2s", recognized: map[string]string{
		"Breaking.Bad.S02E01.1080p.BluRay.x265.10bit.DTS-HD.MA.5.1-ROVERS.mkv": "recognize_breaking_bad.json",
	}, routes: map[string]route{
		searchPath:    ok("search_breaking_bad.json"),
		breakDetails:  ok("detail_breaking_bad.json"),
		seasonsPath:   ok("seasons_breaking_bad.json"),
		breakingQuery: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 绝命毒师")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 2 季")
	h.tap(alice, 1, "从第 1 集开始")
	h.transfers(release("E01", pack, "Breaking.Bad.S02E01.1080p.BluRay.x265.10bit.DTS-HD.MA.5.1-ROVERS.mkv"))
	h.restart()
	h.arrives(alice,
		release("E02", pack, "Breaking.Bad.S02E02.1080p.BluRay.x265.10bit.DTS-HD.MA.5.1-ROVERS.mkv"),
		release("E03", other, "bb-s02e03.mkv"),
	)
	h.shows(2, "E01–E03 到家啦")
	h.shows(2, "1080p · BluRay · x265 10bit · DTS-HD MA 5.1")
	h.tr.verify(t)
}

// Asking for something already subscribed still earns a notice.
func TestAlreadySubscribedRequesterIsNotified(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:  ok("search_dune.json"),
		duneDetails: ok("detail_dune.json"),
		duneLookup:  ok("subscription_existing.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.shows(1, "早就订阅上啦，入库了我第一时间叫你")
	h.arrives(alice, duneFile())
	h.tr.verify(t)
}

// Requests outlive restarts; episodes before the chosen start are ignored,
// and the requester is told when everything they asked for has arrived.
func TestRequestsSurviveRestart(t *testing.T) {
	h := start(t, scenario{routes: conanRoutes()})
	h.say(alice, alice, "/search 名侦探柯南")
	h.tap(alice, 1, conanShow)
	h.tap(alice, 1, conanFirst)
	h.tap(alice, 1, "只追新集（第 1216 集起）")
	h.restart()
	h.arrives(alice,
		conan.file("S01", "E1215"),
		conan.file("S01", "E1216"),
	)
	h.shows(2, "全部到齐啦")
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
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "确认订阅")
	h.transfers(duneFile())
	h.catchUp(alice)
	h.shows(2, "在 Emby 中观看")
	h.tr.verify(t)
}

// A notice telling the arrivals of several watches of one season waits
// until the media server shows all of them: the subscription's E12 shows,
// the download's E01 does not yet, so neither is told before the scan.
func TestSharedNoticeWaitsForEveryArrival(t *testing.T) {
	h := start(t, scenario{lagging: true, quiet: "0s", routes: map[string]route{
		searchPath:      ok("search_mygo.json"),
		mygoDetails:     ok("detail_mygo.json"),
		seasonsPath:     ok("seasons_mygo.json"),
		libraryShowPath: ok("library_mygo.json"),
		mygoLookup:      ok("subscription_none.json"),
		mygoTorrents:    searchingSites("torrents_mygo_s1.json"),
		downloadPath:    ok("download_added.json"),
		subscribePath:   ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 1 季")
	h.searchTorrents(alice, 1, searchResources)
	h.tap(alice, 1, "1")
	h.tap(alice, 1, downloadIt)
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 2, "1")
	h.tap(alice, 2, "第 1 季")
	h.tap(alice, 2, "指定起始集…")
	h.answer(alice, 2, "12")
	h.tap(alice, 2, "确认订阅")
	h.mp.setRoute(libraryShowPath, ok(writeFixture(t, map[string][]int{"1": {12, 13}})))
	early, late := mygo.file("S01", "E01"), mygo.file("S01", "E12")
	early.Hash, late.Hash = addedHash, addedHash
	h.transfers(early, late)
	h.catchUp(alice)
	h.shows(3, "E01、E12 到家啦")
	h.tr.verify(t)
}

// A shared notice says every wanted episode is in only when that holds for
// everyone it tells: Alice's subscription from E13 is complete, Bob's
// download of the season still lacks E02–E12.
func TestSharedNoticeIsCompleteOnlyForEveryWatch(t *testing.T) {
	h := start(t, scenario{quiet: "2s", libraryWait: "0s", routes: map[string]route{
		searchPath: ok("search_mygo.json"), mygoDetails: ok("detail_mygo.json"),
		seasonsPath: ok("seasons_mygo.json"), mygoLookup: ok("subscription_none.json"),
		mygoTorrents: searchingSites("torrents_mygo_s1.json"),
		downloadPath: ok("download_added.json"), subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 1 季")
	h.tap(alice, 1, "指定起始集…")
	h.answer(alice, 1, "13")
	h.tap(alice, 1, "确认订阅")
	h.say(bob, bob, "/search 迷途之子")
	h.tap(bob, 2, "1")
	h.tap(bob, 2, "第 1 季")
	h.searchTorrents(bob, 2, searchResources)
	h.tap(bob, 2, "1")
	h.tap(bob, 2, downloadIt)
	first, final := mygo.file("S01", "E01"), mygo.file("S01", "E13")
	first.Hash, final.Hash = addedHash, addedHash
	told := h.tg.expect(fmt.Sprintf("send:%d", bob))
	h.arrives(alice, first, final)
	h.wait(told, "Bob's share of the notice")
	h.shows(4, "E01、E13 到家啦")
	if strings.Contains(mustMessage(h, 4).text, "全部到齐") {
		t.Error("a notice to a download still lacking episodes says they are all in")
	}
	h.tr.verify(t)
}

// A completed movie does not enter the quiet period; disabling library
// waiting must announce it even when the media server has not scanned it.
func TestDisabledLibraryWaitAnnouncesImmediately(t *testing.T) {
	h := start(t, scenario{lagging: true, quiet: "2s", libraryWait: "0s", routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"),
		duneLookup: ok("subscription_none.json"), subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "确认订阅")
	h.transfers(duneFile())
	h.shows(2, "入库啦")
	h.tr.verify(t)
}

// A requested download that stops moving is told about once: progress
// restarts the clock, and the telling survives a restart, so it is never
// repeated for the same torrent.
func TestStalledDownloadIsToldOnce(t *testing.T) {
	const step = 40 * time.Minute
	h := start(t, scenario{stall: "1h", routes: map[string]route{
		searchPath:      ok("search_mygo.json"),
		mygoDetails:     ok("detail_mygo.json"),
		seasonsPath:     ok("seasons_mygo.json"),
		libraryShowPath: ok("library_mygo.json"),
		mygoLookup:      ok("subscription_none.json"),
		subscribePath:   ok("subscribe_created.json"),
		downloadsPath:   ok("downloads_mygo.json"),
	}})
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 1 季")
	h.tap(alice, 1, "从第 1 集开始")
	h.transfers()
	h.advance(step)
	h.mp.setRoute(downloadsPath, ok("downloads_mygo_later.json"))
	h.transfers()
	h.advance(step)
	h.transfers()
	told := h.tg.expect(fmt.Sprintf("send:%d", alice))
	h.advance(step)
	h.wait(told, "a stalled download notice")
	h.shows(2, "下载好像卡住了")
	h.shows(2, "一直停在 43%")
	h.restart()
	h.transfers()
	h.advance(2 * time.Hour)
	h.transfers()
	h.tr.verify(t)
}

// A download picked for a season that is also subscribed brings files both
// requests wait for: a stuck download and its arrival are each told once.
func TestDownloadOfSubscribedSeasonIsToldOnce(t *testing.T) {
	h := start(t, scenario{stall: "1h", routes: map[string]route{
		searchPath:      ok("search_mygo.json"),
		mygoDetails:     ok("detail_mygo.json"),
		seasonsPath:     ok("seasons_mygo.json"),
		libraryShowPath: ok("library_mygo.json"),
		mygoLookup:      ok("subscription_none.json"),
		mygoTorrents:    searchingSites("torrents_mygo_s1.json"),
		downloadPath:    ok("download_added.json"),
		subscribePath:   ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 1 季")
	h.searchTorrents(alice, 1, searchResources)
	h.tap(alice, 1, "1")
	h.tap(alice, 1, downloadIt)
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 2, "1")
	h.tap(alice, 2, "第 1 季")
	h.tap(alice, 2, "从第 1 集开始")
	h.mp.setRoute(downloadsPath, ok("downloads_mygo_added.json"))
	h.transfers()
	told := h.tg.expect(fmt.Sprintf("send:%d", alice))
	h.advance(2 * time.Hour)
	h.wait(told, "a stalled download notice")
	h.shows(3, "下载好像卡住了")
	pack := mygo.file("S01", "E01-E13")
	pack.Hash = addedHash
	h.arrives(alice, pack)
	h.shows(4, "E01–E13 到家啦")
	h.transfers()
	h.tr.verify(t)
}
