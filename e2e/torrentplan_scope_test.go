package e2e

import (
	"fmt"
	"strings"
	"testing"
)

// The current result filter scopes the plan, pagination does not. Presentation
// sort cannot override H&R/overlap/size/MP-order tie breaking or revive tickets.
func TestTorrentPlanSiteScopeAndStableSelection(t *testing.T) {
	h := start(t, planScenario(t, planPreferenceCandidates(t)))
	planLibrary(h, []int{1, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13})
	h.say(alice, alice, "/search 迷途之子")
	openPlanResources(h, 1)
	h.tap(alice, 1, "筛选站点")
	h.tap(alice, 1, "站点甲")
	if strings.Contains(mustMessage(h, 1).text, "WinnerEarly") {
		t.Fatal("the winning candidate must start outside the current page")
	}
	h.tap(alice, 1, missingPlan)
	h.shows(1, "WinnerEarly")
	h.shows(1, "E03")
	stale, _ := findButton(mustMessage(h, 1).rows, downloadIt)
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "做种")
	h.tap(alice, 1, missingPlan)
	h.shows(1, "WinnerEarly")
	for _, excluded := range []string{"ForeignSite", "WinnerLate", "HRCheap", "RepeatSmall", "UnknownSize", "Larger"} {
		if strings.Contains(mustMessage(h, 1).text, excluded) {
			t.Errorf("plan selected %q instead of the filtered deterministic winner", excluded)
		}
	}
	h.tapData(alice, 1, stale)
	planCalls(h, planCallCounts{searches: 1})
	submitSelected(h, 1, 1)
	planCalls(h, planCallCounts{searches: 1, downloads: 1})
	h.neverShowsSiteCredentials()
	h.tr.verify(t)
}

func planPreferenceCandidates(t *testing.T) []map[string]any {
	t.Helper()
	specs := []planReleaseSpec{
		{name: "HRCheap", episodes: []int{2}, site: "站点甲", size: planGiB / 2, hr: true},
		{name: "RepeatSmall", episodes: []int{1, 2}, site: "站点甲", size: planGiB},
		{name: "UnknownSize", episodes: []int{2}, site: "站点甲"},
		{name: "Larger", episodes: []int{2}, site: "站点甲", size: 4 * planGiB},
	}
	const fillerCount = 4
	for index := range fillerCount {
		specs = append(specs, planReleaseSpec{name: fmt.Sprintf("AlreadyHeld%d", index), episodes: []int{1}, site: "站点甲", size: planGiB})
	}
	specs = append(specs,
		planReleaseSpec{name: "WinnerEarly", episodes: []int{2}, site: "站点甲", size: 2 * planGiB, seeders: 1},
		planReleaseSpec{name: "WinnerLate", episodes: []int{2}, site: "站点甲", size: 2 * planGiB, seeders: 900},
		planReleaseSpec{name: "ForeignSite", episodes: []int{2, 3}, site: "站点乙", size: planGiB},
	)
	var out []map[string]any
	for _, spec := range specs {
		out = append(out, planRelease(t, spec))
	}
	return out
}

// Twenty-one independent gaps cannot produce an unbounded download batch.
// Empty-but-successful inventory stays explicitly caveated, never called proven.
func TestTorrentPlanBoundsAndEmptyInventory(t *testing.T) {
	const seasonTotal, unsupportedSeasonTotal = 24, 101
	h := start(t, planScenario(t, planBoundedCandidates(t)))
	h.mp.setRoute(libraryShowPath, ok(writeFixture(t, map[string][]int{})))
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "第 1 季")
	h.tap(alice, 1, "指定起始集…")
	h.answer(alice, 1, "3")
	h.searchTorrents(alice, 1, searchResources)
	planTotal(h, seasonTotal)
	h.tap(alice, 1, missingPlan)
	h.shows(1, "<td>所需范围</td><td>E03–E24</td>")
	h.shows(1, "<td>起始集以前</td><td>E01（1 集）</td>")
	h.shows(1, "20 个")
	h.shows(1, "<td>仍缺</td><td>E24（1 集）</td>")
	h.shows(1, "未返回本季库存")
	checkTextBudget(t, mustMessage(h, 1).text)
	h.tap(alice, 1, "下一页 ›")
	checkTextBudget(t, mustMessage(h, 1).text)
	planTotal(h, unsupportedSeasonTotal)
	h.tap(alice, 1, replan)
	h.shows(1, "季集范围不确定")
	planNoButton(h, 1, downloadIt)
	planCalls(h, planCallCounts{searches: 1})
	h.tr.verify(t)
}

func planBoundedCandidates(t *testing.T) []map[string]any {
	t.Helper()
	const firstSingleEpisode, lastSingleEpisode = 5, 24
	items := []map[string]any{planRelease(t, planReleaseSpec{name: "BeforeStart", episodes: []int{1, 3, 4}, site: "站点甲", size: planGiB})}
	for episode := firstSingleEpisode; episode <= lastSingleEpisode; episode++ {
		items = append(items, planRelease(t, planReleaseSpec{name: fmt.Sprintf("Single%d", episode), episodes: []int{episode}, site: "站点甲", size: planGiB}))
	}
	return items
}
