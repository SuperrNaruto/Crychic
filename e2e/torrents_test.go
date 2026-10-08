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
	missingOnly     = "只看缺集"
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
// announced when its new files arrive, even if the library held older copies.
func TestDownloadPickedSeasonRelease(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:      ok("search_mygo.json"),
		mygoDetails:     ok("detail_mygo.json"),
		seasonsPath:     ok("seasons_mygo.json"),
		libraryShowPath: ok(writeFixture(t, map[string][]int{"1": {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}})),
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
	first, rest := mygo.file("S01", "E01"), mygo.file("S01", "E02-E13")
	first.Hash, rest.Hash = addedHash, addedHash
	h.arrives(alice, first)
	h.shows(2, "E01 到家啦")
	if strings.Contains(mustMessage(h, 2).text, "全部到齐") {
		t.Error("existing library copies complete a new whole-season download before it arrives")
	}
	h.arrives(alice, rest)
	h.shows(3, "E02–E13 到家啦")
	h.shows(3, "这次下载的剧集全部到齐啦")
	h.neverShowsSiteCredentials()
	h.tr.verify(t)
}

// MoviePilot reads a release's episodes from its description too, so a
// whole-season pack mentioning 「修复第9集章节」 or 「修复第1集字幕」 has
// uncertain episode metadata. With only E01 and E09 in the library
// neither is called 已都有 nor hidden by 只看缺集, since their titles name no
// episode (FLAC.2.0+5.1 and DDP5.1 are audio). Uncertain metadata is never
// presented as a definite range. Missing files complete library coverage,
// but later replacement files from the same download must still be announced.
func TestDownloadMisreadSeasonPack(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:      ok("search_mygo.json"),
		mygoDetails:     ok("detail_mygo.json"),
		seasonsPath:     ok("seasons_mygo.json"),
		libraryShowPath: ok(writeFixture(t, map[string][]int{"1": {1, 9}})),
		mygoLookup:      ok("subscription_none.json"),
		mygoTorrents:    searchingSites("torrents_mygo_misread.json"),
		downloadPath:    ok("download_added.json"),
	}})
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 1 季")
	h.searchTorrents(alice, 1, searchResources)
	releases := mustMessage(h, 1)
	if _, filter := findButton(releases.rows, missingOnly); filter || strings.Contains(releases.text, "已都有") {
		t.Error("a whole-season pack misread from its description counts as bringing nothing missing")
	}
	if !strings.Contains(releases.text, "集数识别不确定") || strings.Contains(releases.text, " · E09") {
		t.Error("uncertain episode metadata is presented as a definite release range")
	}
	h.tap(alice, 1, "1")
	if card := mustMessage(h, 1).text; !strings.Contains(card, "集数识别不确定") || strings.Contains(card, "包含 E09") {
		t.Error("uncertain episode metadata is presented as a definite card range")
	}
	h.tap(alice, 1, downloadIt)
	if strings.Contains(mustMessage(h, 1).text, "第 1 季 E09啦") {
		t.Error("the download receipt promises the incorrectly recognized episode")
	}
	h.restart()
	first, last := mygo.file("S01", "E02-E08"), mygo.file("S01", "E10-E13")
	first.Hash, last.Hash = addedHash, addedHash
	h.arrives(alice, first, last)
	h.shows(2, "E02–E08、E10–E13 到家啦")
	h.shows(2, "加上媒体库已有的，这一季的剧集全部到齐啦")
	h.restart()
	firstHeld, ninthHeld := mygo.file("S01", "E01"), mygo.file("S01", "E09")
	firstHeld.Hash, ninthHeld.Hash = addedHash, addedHash
	h.arrives(alice, firstHeld, ninthHeld)
	h.shows(3, "E01、E09 到家啦")
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

// The releases can be shown in another order, as MoviePilot's WebUI sorts
// them, and for one site only; a number still opens the release it lists.
func TestSortAndFilterReleases(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:   ok("search_dune.json"),
		duneDetails:  ok("detail_dune.json"),
		duneLookup:   ok("subscription_none.json"),
		duneTorrents: searchingSites("torrents_dune.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.searchTorrents(alice, 1, searchResources)
	h.tap(alice, 1, "做种")
	h.shows(1, `<li value="1"><b>Dune.2021.1080p`)
	h.tap(alice, 1, "时间")
	h.shows(1, `<li value="1"><b>Dune.2021.2160p.UHD`)
	h.tap(alice, 1, "筛选站点")
	h.shows(1, "站点乙 · 2 个")
	h.tap(alice, 1, "站点乙")
	h.shows(1, "共 2 个")
	h.tap(alice, 1, "2")
	h.shows(1, "Dune.2021.1080p.BluRay.x264.DTS-HD.MA.5.1-CHD")
	h.tap(alice, 1, "返回")
	h.shows(1, "的资源 · 站点乙")
	h.tap(alice, 1, "筛选站点")
	h.tap(alice, 1, "返回")
	h.shows(1, "共 2 个")
	h.tap(alice, 1, "筛选站点")
	h.tap(alice, 1, "全部站点")
	h.shows(1, "共 4 个")
	if _, offered := findButton(mustMessage(h, 1).rows, missingOnly); offered {
		t.Fatal("a movie's releases offer to show only missing episodes")
	}
	h.tr.verify(t)
}

// A season partly in the library marks what each release adds to it, and
// can list only the releases bringing missing episodes, kept across order
// changes until switched off.
func TestShowReleasesFillingMissingEpisodes(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:      ok("search_mygo.json"),
		mygoDetails:     ok("detail_mygo.json"),
		seasonsPath:     ok("seasons_mygo.json"),
		libraryShowPath: ok("library_mygo.json"),
		mygoLookup:      ok("subscription_none.json"),
		mygoTorrents:    searchingSites("torrents_mygo_s1.json"),
	}})
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 1 季")
	h.searchTorrents(alice, 1, searchResources)
	h.shows(1, "补 E01–E12")
	h.tap(alice, 1, "下一页 ›")
	h.shows(1, "已都有")
	h.tap(alice, 1, missingOnly)
	h.shows(1, "的资源 · 缺集")
	h.shows(1, "共 13 个")
	h.tap(alice, 1, "做种")
	h.shows(1, "共 13 个")
	h.tap(alice, 1, "·"+missingOnly+"·")
	h.shows(1, "共 14 个")
	h.tr.verify(t)
}

// Several releases can be ticked across pages and orders and downloaded
// with one confirmation, each once; every download is watched on its own,
// and a result MoviePilot leaves unknown points to /tasks.
func TestDownloadTickedReleases(t *testing.T) {
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
	h.tap(alice, 1, "多选…")
	h.tap(alice, 1, "2")
	h.tap(alice, 1, "3")
	h.tap(alice, 1, "下一页 ›")
	h.tap(alice, 1, "9")
	h.shows(1, "第 2/2 页")
	h.tap(alice, 1, "做种")
	h.tap(alice, 1, "默认")
	h.tap(alice, 1, "✓ 3")
	h.shows(1, "已选 2 个")
	h.tap(alice, 1, "下载所选 2 个")
	h.shows(1, "要下载这 2 个资源吗？")
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "下载所选 2 个")
	confirm, _ := findButton(mustMessage(h, 1).rows, downloadIt)
	h.tap(alice, 1, downloadIt)
	h.shows(1, "开始下载 2 个资源啦")
	h.tapData(alice, 1, confirm)
	first := mygo.file("S01", "E01")
	first.Hash = addedHash
	h.arrives(alice, first)
	eighth := mygo.file("S01", "E08")
	eighth.Hash = addedHashN(2)
	h.arrives(alice, eighth)

	h.mp.setRoute(downloadPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 4, "1")
	h.tap(alice, 4, "第 1 季")
	h.searchTorrents(alice, 4, searchResources)
	h.tap(alice, 4, "多选…")
	h.tap(alice, 4, "1")
	h.tap(alice, 4, "下载所选 1 个")
	h.tap(alice, 4, downloadIt)
	h.shows(4, "/tasks")
	if _, offered := findButton(mustMessage(h, 4).rows, downloadIt); offered {
		t.Fatal("a download of unknown result is offered again")
	}
	h.neverShowsSiteCredentials()
	h.tr.verify(t)
}

// A title searched with 下载 searches resources as soon as a target is
// chosen, and 返回 leads from the releases to the card it skipped; 订阅
// and 搜索 only search, as MoviePilot's own bot reads them. A title typed in
// answer to 搜索 reads 下载 the same way and shows the search's outcome.
func TestSearchPrefixes(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:      ok("search_mygo.json"),
		mygoDetails:     ok("detail_mygo.json"),
		seasonsPath:     ok("seasons_mygo.json"),
		libraryShowPath: ok("library_mygo.json"),
		mygoLookup:      ok("subscription_none.json"),
		mygoTorrents:    searchingSites("torrents_mygo_s1.json"),
	}})
	h.say(alice, alice, "下载 迷途之子")
	h.tap(alice, 1, "1")
	h.searchTorrents(alice, 1, "第 1 季")
	h.shows(1, "《迷途之子!!!!!》第 1 季的资源")
	h.tap(alice, 1, "返回")
	h.shows(1, "要订阅《迷途之子!!!!!》第 1 季")
	h.tap(alice, 1, "返回")
	h.shows(1, "第 1 季")
	h.say(alice, alice, "订阅：迷途之子")
	h.tap(alice, 2, "1")
	h.tap(alice, 2, "第 1 季")
	h.shows(2, "要订阅《迷途之子!!!!!》第 1 季")

	h.mp.setRoute(searchPath, ok("search_abyss.json"))
	h.mp.setRoute(seasonsPath, ok("seasons_abyss.json"))
	h.mp.setRoute(abyssDetails, ok("detail_abyss.json"))
	h.mp.setRoute(abyssLookup, ok("subscription_none.json"))
	h.mp.setRoute("GET /api/v1/search/media/301489", searchingSites("torrents_none.json"))
	searched := h.tg.expect("edit:3")
	h.say(alice, alice, "下载：深渊")
	h.wait(searched, "a resource search from the first message")
	h.shows(3, "未搜索到任何资源")
	h.say(alice, alice, "/start")
	h.tap(alice, 4, "搜索")
	h.answer(alice, 4, "下载：深渊")
	h.wait(h.tg.expect("edit:4"), "a resource search from a typed answer")
	h.shows(4, "未搜索到任何资源")
	h.tr.verify(t)
}

// 下载 outlasts a failed search: after 重试, picking a movie already in the
// library still searches its resources at once, and once the release is
// downloaded 返回 leads back to the results it was picked from.
func TestDownloadPrefixSurvivesRetryAndLeadsBackToResults(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:       {status: http.StatusServiceUnavailable, fixture: "server_error.json"},
		duneDetails:      ok("detail_dune.json"),
		libraryMoviePath: ok("library_movie_held.json"),
		duneLookup:       ok("subscription_none.json"),
		duneTorrents:     searchingSites("torrents_dune.json"),
		downloadPath:     ok("download_added.json"),
	}})
	h.say(alice, alice, "/search 下载 沙丘")
	h.mp.setRoute(searchPath, ok("search_dune.json"))
	h.tap(alice, 1, "重试")
	h.searchTorrents(alice, 1, duneMovie)
	h.shows(1, "《沙丘》的资源")
	h.tap(alice, 1, "返回")
	h.shows(1, "已经在媒体库里啦")
	h.searchTorrents(alice, 1, searchResources)
	h.tap(alice, 1, "1")
	h.tap(alice, 1, downloadIt)
	h.shows(1, "开始下载《沙丘》啦")
	h.tap(alice, 1, "返回")
	h.shows(1, "帮你找到这些「沙丘」啦")
	h.tr.verify(t)
}

// 首页 ends a completed title search's download intent: a subsequent
// discovery pick asks to subscribe, without searching its resources.
func TestHomeAfterDownloadReturnsToBrowsing(t *testing.T) {
	const diggerTorrents = "GET /api/v1/search/media/1248832"
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok(selectedFixture(t, "search_dune.json", 0, 3)),
		duneDetails:   ok("detail_dune.json"),
		duneLookup:    ok("subscription_none.json"),
		duneTorrents:  searchingSites("torrents_dune.json"),
		downloadPath:  ok("download_added.json"),
		trendingPath:  ok(selectedFixture(t, "chart_trending.json", 0)),
		diggerDetails: ok("detail_digger.json"),
		diggerLookup:  ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "下载 沙丘")
	h.searchTorrents(alice, 1, duneMovie)
	h.tap(alice, 1, "1")
	h.tap(alice, 1, downloadIt)
	h.tap(alice, 1, "首页")
	h.tap(alice, 1, "发现")
	h.mp.setRoute(searchPath, ok("search_digger.json"))
	h.tap(alice, 1, "TMDB 流行趋势")
	h.tap(alice, 1, "1")
	h.shows(1, "要订阅《挖掘者》")
	h.tap(alice, 1, "确认订阅")
	h.shows(1, "帮你订好《挖掘者》")
	if strings.Contains(h.tr.String(), diggerTorrents) {
		t.Error("a discovery pick searched resources without being asked")
	}
	h.tr.verify(t)
}
