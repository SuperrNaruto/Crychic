package e2e

import (
	"encoding/json"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Direct requests keep their terminal receipt: duplicate confirmation and
// old navigation cannot change it or start another write, until expiry.
func TestDirectRequestKeepsItsReceipt(t *testing.T) {
	cases := []struct {
		name, start, search, confirm string
		steps                        []string
	}{
		{"movie", "沙丘", "search_dune.json", "确认订阅", nil},
		{"deep_link", "/start m_mv_t_438631", "search_dune.json", "确认订阅", nil},
		{"upgrade", "沙丘", "search_dune.json", "确认洗版", []string{"洗版订阅"}},
		{"seasons", "绝命毒师", "search_breaking_bad.json", "订阅所选 2 季", []string{"多选季…", "第 2 季", "第 3 季"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := start(t, scenario{routes: map[string]route{
				searchPath:  ok(selectedFixture(t, tc.search, 0)),
				duneDetails: ok("detail_dune.json"), duneLookup: ok("subscription_none.json"),
				breakDetails: ok("detail_breaking_bad.json"), breakingQuery: ok("subscription_none.json"),
				seasonsPath: ok("seasons_breaking_bad.json"), subscribePath: ok("subscribe_created.json"),
			}})
			h.say(alice, alice, tc.start)
			oldCancel, _ := findButton(mustMessage(h, 1).rows, "取消")
			for _, step := range tc.steps {
				h.tap(alice, 1, step)
			}
			confirm, _ := findButton(mustMessage(h, 1).rows, tc.confirm)
			h.tap(alice, 1, tc.confirm)
			receipt := mustMessage(h, 1).seen()
			writes := strings.Count(h.tr.String(), subscribePath)
			h.tapData(alice, 1, confirm)
			h.tapData(alice, 1, oldCancel)
			if mustMessage(h, 1).seen() != receipt || strings.Count(h.tr.String(), subscribePath) != writes {
				t.Fatal("a stale direct-request button changed its receipt or wrote again")
			}
			const beyondSession = 11 * time.Minute
			h.advance(beyondSession)
			h.tapData(alice, 1, confirm)
			h.shows(1, "过期啦")
			h.tr.verify(t)
		})
	}
}

// A repeated list pick must not push the same card twice onto 返回 history.
func TestRepeatedPickKeepsOneBackStep(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"), duneLookup: ok("subscription_none.json"),
	}})
	h.say(alice, alice, "沙丘")
	pick, _ := findButton(mustMessage(h, 1).rows, duneMovie)
	h.tap(alice, 1, duneMovie)
	h.tapData(alice, 1, pick)
	h.tap(alice, 1, "返回")
	h.shows(1, "这些「沙丘」啦")
	h.tr.verify(t)
}

// Pausing or resuming updates the list returned to from details, keeping
// the filter and page where the user opened the subscription.
func TestSubscriptionStateReturnsToCurrentList(t *testing.T) {
	h := requestedBreakingSeasons(t)
	const firstListed = 21
	var subs []map[string]any
	for id := 1; id < firstListed; id++ {
		subs = append(subs, breakingSeason(firstListed+id, 4))
	}
	subs = append(subs, breakingSeason(1, 2), breakingSeason(2, 3))
	h.mp.setRoute(subsPath, ok(writeFixture(t, subs)))
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 3, "下一页 ›")
	h.tap(alice, 3, "21")
	h.tap(alice, 3, "暂停订阅")
	h.tap(alice, 3, "返回")
	h.shows(3, "已暂停")
	h.shows(3, "第 2/2 页")
	h.tap(alice, 3, "21")
	h.tap(alice, 3, "恢复订阅")
	h.tap(alice, 3, "返回")
	h.shows(3, "订阅清单 · 电视剧")
	h.shows(3, "第 2/2 页")
	if strings.Contains(mustMessage(h, 3).text, "已暂停") {
		t.Fatal("the resumed subscription remains paused in its list")
	}
	h.tap(alice, 3, "22")
	h.mp.setRoute("PUT /api/v1/subscribe/status/2", route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, 3, "暂停订阅")
	subs[firstListed]["state"] = "S"
	h.mp.setRoute(subsPath, ok(writeFixture(t, subs)))
	h.tap(alice, 3, "刷新")
	h.shows(3, "已暂停")
	h.tap(alice, 3, "返回")
	h.shows(3, "已暂停")
	h.shows(3, "第 2/2 页")
	h.tr.verify(t)
}

// Generic paging/back buttons occur in several features. After expiry
// they offer a neutral home entry instead of misidentifying a title search.
func TestExpiredBrowsingKeepsNeutralRecovery(t *testing.T) {
	const listCount = 21
	cases := []struct {
		name, command, path, fixture, pick, press string
	}{
		{"subscriptions_page", "/subscribe", subsPath, manySubscriptions(t, listCount), "", "下一页 ›"},
		{"subscription_back", "/subscribe", subsPath, "subscriptions.json", "1", "返回"},
		{"tasks_page", "/tasks", downloadsPath, manyNavigationDownloads(t, listCount), "", "下一页 ›"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := start(t, scenario{routes: map[string]route{tc.path: ok(tc.fixture)}})
			h.say(alice, alice, tc.command)
			if tc.pick != "" {
				h.tap(alice, 1, tc.pick)
			}
			const beyondSession = 11 * time.Minute
			h.advance(beyondSession)
			h.tap(alice, 1, tc.press)
			h.shows(1, "这个页面过期啦")
			h.shows(1, "/start")
			if strings.Contains(mustMessage(h, 1).text, "/search") {
				t.Fatal("an expired browsing page was mistaken for a title search")
			}
			h.tr.verify(t)
		})
	}
}

func manyNavigationDownloads(t *testing.T, count int) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "moviepilot", "downloads_mygo.json"))
	if err != nil {
		t.Fatal(err)
	}
	var env struct{ Data []map[string]any }
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	var downloads []map[string]any
	for i := range count {
		download := maps.Clone(env.Data[0])
		download["hash"] = "navigation-" + strconv.Itoa(i)
		downloads = append(downloads, download)
	}
	return writeFixture(t, downloads)
}
