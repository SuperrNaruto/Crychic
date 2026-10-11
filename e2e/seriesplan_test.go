package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	dune3Lookup    = "GET /api/v1/subscribe/media/1170608"
	seriesPlanText = "补齐缺少的作品…"
)

// A series lists where each part stands. While the subscriptions cannot be
// read, only what the library shows is known and nothing can be ticked;
// once they read, the missing part is ticked and subscribed once, a stale
// confirmation sends nothing, and the outcome is tabled per part.
func TestSeriesFillsInMissingParts(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"), duneLookup: ok("subscription_none.json"),
		collectionSearch("沙丘"): ok("collections_dune.json"), duneCollection: ok("collection_dune.json"),
		subsPath:    route{status: http.StatusServiceUnavailable, fixture: "server_error.json"},
		dune3Lookup: ok("subscription_none.json"), subscribePath: ok("subscribe_created.json"),
	}})
	h.transfers(duneFile())
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "同系列")
	h.shows(1, "部分作品的状态暂时无法确认")
	if _, offered := findButton(mustMessage(h, 1).rows, seriesPlanText); offered {
		t.Error("parts of unknown state can be ticked to subscribe")
	}
	h.tap(alice, 1, "返回")
	h.mp.setRoute(subsPath, ok(subscribedDunePartTwo(t)))
	h.tap(alice, 1, "同系列")
	text := mustMessage(h, 1).text
	for _, state := range []string{"已入库", "已订阅", "未获取"} {
		if !strings.Contains(text, state) {
			t.Errorf("the series list does not say %s:\n%s", state, text)
		}
	}
	h.tap(alice, 1, seriesPlanText)
	h.tap(alice, 1, "3")
	confirm, _ := findButton(mustMessage(h, 1).rows, "订阅所选 1 部")
	h.tap(alice, 1, "订阅所选 1 部")
	h.shows(1, "<td><b>《沙丘3》</b></td><td>✅ 已订阅</td>")
	h.tapData(alice, 1, confirm)
	h.tr.verify(t)
}

// subscribedDunePartTwo is the subscription list with 沙丘2 subscribed.
func subscribedDunePartTwo(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "moviepilot", "subscriptions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var env struct{ Data []map[string]any }
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	sub := env.Data[0]
	sub["id"], sub["name"], sub["type"], sub["media_id"], sub["year"], sub["season"] = 9, "沙丘2", "电影", "693134", "2024", nil
	return writeFixture(t, env.Data)
}
