package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const (
	showResources  = "GET /api/v1/subscribe/files/1"
	movieResources = "GET /api/v1/subscribe/files/6"
	resourceHash   = "synthetic-resource-hash"
)

// The subscription's own file records determine resources, not matching
// titles. A pack shared by episodes appears once; live size/progress are
// cosmetic, and only basenames (not credentials or server paths) are shown.
func TestSubscriptionResources(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		subsPath:       ok("subscriptions.json"),
		showResources:  ok("resources_show.json"),
		movieResources: ok("resources_movie.json"),
		downloadsPath:  ok("downloads_resources.json"),
	}})
	h.say(alice, group, "/subscribe")
	h.tap(bob, 1, "1")
	h.tap(alice, 1, "1")
	h.shows(1, "Example.Show.S02.1080p.WEB-DL.H265")
	h.shows(1, "E01–E02")
	h.shows(1, "43%")
	h.shows(1, "2.0 GiB")
	h.shows(1, "Example.Show.S02E01.mkv")
	h.mp.setRoute(downloadsPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, 1, "刷新")
	h.shows(1, "下载进度暂时查不到")
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "3")
	h.shows(1, "Dune.2021.2160p")
	h.shows(1, "Dune.2021.mkv")
	h.tr.verify(t)
}

// An empty response and failed reads keep the list usable; selecting
// again retries only the read, with no new subscription or torrent search.
func TestSubscriptionResourceReadsCanRecover(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		subsPath:      ok("subscriptions.json"),
		showResources: ok("resources_empty.json"),
	}})
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 1, "1")
	h.mp.setRoute(showResources, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, 1, "1")
	h.mp.setRoute(showResources, ok("resources_unavailable.json"))
	h.tap(alice, 1, "1")
	h.mp.setRoute(showResources, ok("resources_show.json"))
	h.tap(alice, 1, "1")
	h.shows(1, "Example.Show.S02.1080p")
	h.tr.verify(t)
}

// Large packs and resource lists stay bounded, with later entries reachable.
func TestSubscriptionResourcePages(t *testing.T) {
	const count = 25
	const longFieldRepeats = 80
	const tableFieldRepeats = 50
	var files []map[string]any
	for i := range count {
		files = append(files, map[string]any{
			"hash": fmt.Sprint(i), "torrent_title": fmt.Sprintf("Release-%02d", i),
			"file_path":  "/private/path/" + strings.Repeat("长文件名", longFieldRepeats) + ".mkv",
			"site_name":  strings.Repeat("站点", tableFieldRepeats),
			"downloader": strings.Repeat("下载", tableFieldRepeats),
		})
	}
	pack := append([]map[string]any{}, files...)
	for i := range pack {
		pack[i] = map[string]any{"hash": resourceHash, "torrent_title": "Season pack", "file_path": fmt.Sprintf("/private/path/E%02d.mkv", i+1)}
	}
	files = append(files, pack...)
	fixture := writeFixture(t, map[string]any{
		"subscribe": map[string]any{"id": 1},
		"episodes":  map[string]any{"1": map[string]any{"download": files}},
	})
	h := start(t, scenario{routes: map[string]route{subsPath: ok("subscriptions.json"), showResources: ok(fixture)}})
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 1, "1")
	for {
		msg := mustMessage(h, 1)
		checkTextBudget(t, msg.text)
		if _, more := findButton(msg.rows, "下一页 ›"); !more {
			break
		}
		h.tap(alice, 1, "下一页 ›")
	}
	h.shows(1, "Season pack")
	h.shows(1, "还有")
	h.tap(alice, 1, "返回")
	h.shows(1, "订阅清单")
	h.tr.verify(t)
}
