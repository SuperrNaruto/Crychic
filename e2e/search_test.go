package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const abyssLookup = "GET /api/v1/subscribe/media/301489"

// All returned results are reachable; returning from a pick restores its
// page without another search.
func TestSearchPagesKeepLastResultAndBack(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:                           ok("search_dune.json"),
		"GET /api/v1/subscribe/media/641213": ok("subscription_none.json"),
	}})
	h.say(alice, alice, "沙丘")
	h.tap(alice, 1, "下一页 ›")
	h.shows(1, `<li value="10">`)
	last := mustMessage(h, 1)
	h.tap(alice, 1, "10")
	h.tap(alice, 1, "返回")
	h.shows(1, "第 2/2 页")
	if mustMessage(h, 1).text != last.text {
		t.Fatal("return did not restore the search page")
	}
	h.tap(alice, 1, "‹ 上一页")
	h.shows(1, "第 1/2 页")
	h.tr.verify(t)
}

// A single movie opens directly, but changing the title starts a fresh
// conversation in the same message and invalidates its old confirmation.
func TestResearchReplacesOldConfirmation(t *testing.T) {
	h := start(t, scenario{
		routes: map[string]route{
			duneDetails: ok("detail_dune.json"),
			duneLookup:  ok("subscription_none.json"),
		},
		searches: map[string]string{
			"沙丘":    selectedFixture(t, "search_dune.json", 0),
			"名侦探柯南": "search_conan.json",
		},
	})
	h.say(alice, alice, "沙丘")
	h.shows(1, "要订阅《沙丘》")
	old, _ := findButton(mustMessage(h, 1).rows, "确认订阅")
	h.tap(alice, 1, "重新搜索")
	h.tap(alice, 1, "返回")
	h.shows(1, "要订阅《沙丘》")
	h.tap(alice, 1, "重新搜索")
	h.answer(alice, 1, "名侦探柯南")
	h.shows(1, "这些「名侦探柯南」啦")
	h.tapData(alice, 1, old)
	h.shows(1, "过期啦")
	h.tr.verify(t)
}

// Neither a redundant result list nor a redundant season picker is shown,
// while subscribing still requires the final explicit choice.
func TestSingleResultAndSeasonGoStraightToConfirm(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:    ok("search_abyss.json"),
		abyssDetails:  ok("detail_abyss.json"),
		seasonsPath:   ok("seasons_abyss.json"),
		abyssLookup:   ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "深渊无间")
	h.shows(1, "要订阅《深渊无间》第 1 季")
	h.tap(alice, 1, "从第 1 集开始")
	h.shows(1, "帮你订好《深渊无间》第 1 季")
	h.tr.verify(t)
}

// A special is never chosen implicitly, even if it is the only season.
func TestSingleSpecialNeedsExplicitChoice(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:   ok("search_abyss.json"),
		abyssDetails: ok("detail_abyss.json"),
		seasonsPath:  ok(selectedFixture(t, "seasons_conan.json", 0)),
		abyssLookup:  ok("subscription_none.json"),
	}})
	h.say(alice, alice, "深渊无间")
	h.shows(1, "想订哪一季呀")
	h.tap(alice, 1, "特别篇")
	h.shows(1, "要订阅")
	h.tap(alice, 1, "取消")
	h.tr.verify(t)
}

// Keep recorded service fields intact when a scenario needs fewer results.
func selectedFixture(t *testing.T, name string, indices ...int) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "moviepilot", name))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct{ Data []json.RawMessage }
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	var selected []json.RawMessage
	for _, index := range indices {
		selected = append(selected, envelope.Data[index])
	}
	return writeFixture(t, selected)
}
