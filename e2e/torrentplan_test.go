package e2e

import (
	"net/http"
	"strings"
	"testing"
)

const (
	missingPlan = "缺集方案"
	replan      = "重新计算"
)

// Independent owner of trusted coverage and partial planning. Unsafe candidates
// offer more missing episodes, so a wrong acceptance cannot pass by losing a tie.
func TestTorrentPlanCoverageAndMixedReceipts(t *testing.T) {
	h := start(t, planScenario(t, planCoverageCandidates(t)))
	planLibrary(h, []int{1, 6, 7, 8, 9, 10, 11, 12, 13})
	h.say(alice, alice, "/search 迷途之子")
	openPlanResources(h, 1)
	h.tap(alice, 1, missingPlan)
	for _, text := range []string{"所需", "缺集", "可补", "仍缺", "E02", "CoverA", "CoverB", "重复已有", "方案内重叠", "大小未知", "H&amp;R", "未报告做种"} {
		h.shows(1, text)
	}
	for _, excluded := range []string{"LargerTie", "UnknownPack", "OutOfBounds", "UnsurePack", "Wrong", "UnknownSeason", "MultipleSeasons", "MetaGroup", "MediaGroup"} {
		if strings.Contains(mustMessage(h, 1).text, excluded) {
			t.Errorf("unsafe or lower-priority candidate %q entered the plan", excluded)
		}
	}
	confirm, _ := findButton(mustMessage(h, 1).rows, downloadIt)
	calls := h.mp.holdCalls(downloadPath, 2)
	h.tap(alice, 1, downloadIt)
	h.wait(calls[0].entered, "the first planned download")
	calls[0].allow()
	h.wait(calls[1].entered, "the second planned download")
	batchShows(h, 1, "正在提交 1/2")
	h.mp.setRoute(downloadPath, ok("download_rejected.json"))
	calls[1].allow()
	batchShows(h, 1, "下载结果来啦")
	h.shows(1, "开始下载 1 个资源")
	h.shows(1, "任务添加失败")
	h.tapData(alice, 1, confirm)
	planCalls(h, planCallCounts{searches: 1, downloads: 2})
	h.neverShowsSiteCredentials()
	assertResourcePrivacy(h)
	h.tr.verify(t)
}

// Read errors and unknown season totals never become an invented full range;
// retrying only the plan reuses the original, already completed site search.
func TestTorrentPlanMetadataRecovery(t *testing.T) {
	items := []map[string]any{planRelease(t, planReleaseSpec{name: "MissingTwo", episodes: []int{2}, site: "站点甲", size: planGiB})}
	h := start(t, planScenario(t, items))
	h.say(alice, alice, "/search 迷途之子")
	openPlanResources(h, 1)
	h.mp.setRoute(libraryShowPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, 1, missingPlan)
	h.shows(1, "没能读取媒体库")
	planNoButton(h, 1, downloadIt)
	planLibrary(h, []int{1, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13})
	h.mp.setRoute(seasonsPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, 1, replan)
	h.shows(1, "没能读取季信息")
	planNoButton(h, 1, downloadIt)
	planTotal(h, 0)
	h.tap(alice, 1, replan)
	h.shows(1, "季集范围不确定")
	planNoButton(h, 1, downloadIt)
	h.mp.setRoute(seasonsPath, ok(writeFixture(t, []any{nil})))
	h.tap(alice, 1, replan)
	planNoButton(h, 1, downloadIt)
	planTotal(h, planSeasonEpisodes)
	planNullInventory(h)
	planLibrary(h, []int{1, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13})
	h.tap(alice, 1, replan)
	h.shows(1, "MissingTwo")
	h.tap(alice, 1, "返回")
	planLibrary(h, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13})
	h.tap(alice, 1, missingPlan)
	planNoButton(h, 1, downloadIt)
	planCalls(h, planCallCounts{searches: 1})
	h.tr.verify(t)
}

func planNullInventory(h *harness) {
	h.t.Helper()
	for _, body := range []any{nil, map[string]any{"1": nil}} {
		h.mp.setRoute(libraryShowPath, ok(writeFixture(h.t, body)))
		h.tap(alice, 1, replan)
		h.shows(1, "没能读取媒体库")
		planNoButton(h, 1, downloadIt)
	}
}

// A valid ticket cannot write when its inventory or season snapshot changed.
// A failed recheck consumes no write and cannot revive an earlier ticket.
func TestTorrentPlanRechecksBeforeConfirm(t *testing.T) {
	items := []map[string]any{
		planRelease(t, planReleaseSpec{name: "MissingTwo", episodes: []int{2}, site: "站点甲", size: planGiB}),
		planRelease(t, planReleaseSpec{name: "MissingThree", episodes: []int{3}, site: "站点甲", size: planGiB}),
	}
	h := start(t, planScenario(t, items))
	planLibrary(h, []int{1, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13})
	h.say(alice, alice, "/search 迷途之子")
	openPlanResources(h, 1)
	h.tap(alice, 1, missingPlan)
	first, _ := findButton(mustMessage(h, 1).rows, downloadIt)
	planLibrary(h, []int{1, 2, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13})
	h.tap(alice, 1, downloadIt)
	h.shows(1, "库存或季集范围已变化")
	h.shows(1, "MissingThree")
	second, _ := findButton(mustMessage(h, 1).rows, downloadIt)
	planTotal(h, 14)
	h.tap(alice, 1, downloadIt)
	h.shows(1, "库存或季集范围已变化")
	h.shows(1, "E14")
	third, _ := findButton(mustMessage(h, 1).rows, downloadIt)
	h.mp.setRoute(libraryShowPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, 1, downloadIt)
	h.shows(1, "没能读取媒体库")
	planNoButton(h, 1, downloadIt)
	planCalls(h, planCallCounts{searches: 1})
	planLibrary(h, []int{1, 2, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13})
	h.tapData(alice, 1, third)
	h.tap(alice, 1, replan)
	current, _ := findButton(mustMessage(h, 1).rows, downloadIt)
	for _, stale := range []string{first, second, third} {
		h.tapData(alice, 1, stale)
	}
	planCalls(h, planCallCounts{searches: 1})
	submitSelected(h, 1, 1)
	h.tapData(alice, 1, current)
	planCalls(h, planCallCounts{searches: 1, downloads: 1})
	h.tr.verify(t)
}
