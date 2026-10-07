package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	duneTorrents = "GET /api/v1/search/media/438631"
	mygoTorrents = "GET /api/v1/search/media/224207"

	searchResources = "搜索资源"
	downloadIt      = "确认下载"
	// addedHash is the download id download_added.json answers with.
	addedHash = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b"
	// siteSearch is how long the fake takes to search the sites, so the
	// search outlasts the button press like a real one does.
	siteSearch = 200 * time.Millisecond
	// searchedFor is how long a scenario lets the sites be searched.
	searchedFor = 20 * time.Second
)

func searchingSites(fixture string) route {
	return route{status: http.StatusOK, fixture: fixture, delay: siteSearch}
}

// searchTorrents presses 搜索资源 on message msgID and waits for the search
// that runs after the press to show its outcome.
func (h *harness) searchTorrents(user int64, msgID int, label string) {
	h.t.Helper()
	h.tap(user, msgID, label)
	h.wait(h.tg.expect(fmt.Sprintf("edit:%d", msgID)), "a resource search")
}

// neverShowsSiteCredentials fails if a site cookie or passkey from a search
// result reached a chat.
func (h *harness) neverShowsSiteCredentials() {
	h.t.Helper()
	if text := h.tr.String(); strings.Contains(text, "fake-cookie") || strings.Contains(text, "fake-passkey") {
		h.t.Fatalf("site credentials reached the transcript:\n%s", text)
	}
}

// A movie's releases are searched from its card while the message says so;
// the picked release is downloaded with the search result handed back
// unchanged, and its requester hears when that download's file arrives.
func TestDownloadPickedMovieRelease(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:   ok("search_dune.json"),
		duneDetails:  ok("detail_dune.json"),
		duneLookup:   ok("subscription_none.json"),
		duneTorrents: searchingSites("torrents_dune.json"),
		downloadPath: ok("download_added.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.searchTorrents(alice, 1, searchResources)
	h.shows(1, "Dune.2021.2160p.UHD.BluRay.REMUX")
	h.tap(alice, 1, "1")
	h.shows(1, "H&amp;R 资源")
	h.tap(alice, 1, downloadIt)
	h.shows(1, "开始下载《沙丘》啦")
	file := duneFile()
	file.Hash = addedHash
	h.arrives(alice, file)
	h.shows(2, "入库啦")
	h.neverShowsSiteCredentials()
	h.tr.verify(t)
}

// A season's releases page like other lists and 返回 leads back to the page
// picked from; a double tap downloads once, and a whole-season release is
// announced when its files arrive.
func TestDownloadPickedSeasonRelease(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:      ok("search_mygo.json"),
		mygoDetails:     ok("detail_mygo.json"),
		seasonsPath:     ok("seasons_mygo.json"),
		libraryShowPath: ok("library_mygo.json"),
		mygoLookup:      ok("subscription_none.json"),
		mygoTorrents:    searchingSites("torrents_mygo_s1.json"),
		downloadPath:    ok("download_added.json"),
	}})
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 1 季")
	h.searchTorrents(alice, 1, searchResources)
	h.shows(1, "第 1/2 页")
	h.tap(alice, 1, "下一页 ›")
	h.tap(alice, 1, "9")
	h.shows(1, "包含 E08")
	h.tap(alice, 1, "返回")
	h.shows(1, "第 2/2 页")
	h.tap(alice, 1, "‹ 上一页")
	h.tap(alice, 1, "1")
	confirm, _ := findButton(mustMessage(h, 1).rows, downloadIt)
	h.tap(alice, 1, downloadIt)
	h.shows(1, "开始下载《迷途之子!!!!!》第 1 季啦")
	h.tapData(alice, 1, confirm)
	pack := mygo.file("S01", "E01-E13")
	pack.Hash = addedHash
	h.arrives(alice, pack)
	h.neverShowsSiteCredentials()
	h.tr.verify(t)
}

// A failed search keeps the card and can be repeated; MoviePilot's own
// words for nothing found and for a refused download reach the user, and a
// download is never offered again, only checked for with /tasks.
func TestResourceSearchAndDownloadFailures(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:   ok("search_dune.json"),
		duneDetails:  ok("detail_dune.json"),
		duneLookup:   ok("subscription_none.json"),
		duneTorrents: route{status: http.StatusServiceUnavailable, fixture: "server_error.json", delay: siteSearch},
		downloadPath: ok("download_rejected.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.searchTorrents(alice, 1, searchResources)
	h.shows(1, "MoviePilot 好像打了个盹")
	h.mp.setRoute(duneTorrents, searchingSites("torrents_none.json"))
	h.searchTorrents(alice, 1, "重试")
	h.shows(1, "未搜索到任何资源")
	h.mp.setRoute(duneTorrents, searchingSites("torrents_dune.json"))
	h.searchTorrents(alice, 1, "重试")
	h.tap(alice, 1, "2")
	h.tap(alice, 1, downloadIt)
	h.shows(1, "⚠️ 任务添加失败")
	if _, offered := findButton(mustMessage(h, 1).rows, downloadIt); offered {
		t.Fatal("a refused download is offered again")
	}

	h.mp.setRoute(downloadPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 2, duneMovie)
	h.searchTorrents(alice, 2, searchResources)
	h.tap(alice, 2, "2")
	h.tap(alice, 2, downloadIt)
	h.shows(2, "/tasks")
	h.tr.verify(t)
}

// While the sites are searched the card says for how long, and cancelling
// stops the search at once.
func TestResourceSearchShowsElapsedAndCancels(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:   ok("search_dune.json"),
		duneDetails:  ok("detail_dune.json"),
		duneLookup:   ok("subscription_none.json"),
		duneTorrents: route{status: http.StatusOK, fixture: "torrents_dune.json", delay: time.Minute},
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, searchResources)
	h.shows(1, "要等一会儿哦")
	refreshed := h.tg.expect("edit:1")
	h.advance(searchedFor)
	h.wait(refreshed, "the elapsed search time")
	h.shows(1, "已经搜了 20 秒")
	h.tap(alice, 1, "取消")
	h.shows(1, "已经取消啦")
	h.tr.verify(t)
}
