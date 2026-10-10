package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	planSeasonEpisodes = 13
	planGiB            = 1 << 30
	planStream         = "/api/v1/search/media/224207/stream"
)

type planReleaseSpec struct {
	name     string
	episodes []int
	site     string
	size     float64
	seeders  int
	hr       bool
}

type planCallCounts struct{ searches, downloads int }

// Every candidate copies an existing synthetic MP Context before mutation.
// Only upstream inputs are changed; no expected answer is calculated here.
func planRelease(t *testing.T, spec planReleaseSpec) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "moviepilot", "torrents_mygo_s1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var env struct{ Data []map[string]any }
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	item := env.Data[1]
	meta := item["meta_info"].(map[string]any)
	meta["episode_list"] = spec.episodes
	meta["total_episode"] = len(spec.episodes)
	meta["begin_episode"], meta["end_episode"] = nil, nil
	var labels []string
	for _, episode := range spec.episodes {
		labels = append(labels, fmt.Sprintf("E%02d", episode))
	}
	title := "MyGO.S01." + strings.Join(labels, ".") + ".1080p-" + spec.name
	meta["title"], meta["org_string"] = title, title
	meta["subtitle"], meta["season_episode"] = "合成资源", "S01 "+strings.Join(labels, "、")
	torrent := item["torrent_info"].(map[string]any)
	torrent["title"], torrent["description"] = title, "合成资源"
	torrent["site_name"], torrent["size"] = spec.site, spec.size
	torrent["seeders"], torrent["hit_and_run"] = spec.seeders, spec.hr
	return item
}

func planScenario(t *testing.T, items []map[string]any) scenario {
	t.Helper()
	return scenario{stall: "0s", routes: map[string]route{
		searchPath: ok("search_mygo.json"), mygoDetails: ok("detail_mygo.json"),
		seasonsPath: ok("seasons_mygo.json"), mygoLookup: ok("subscription_none.json"),
		libraryShowPath: ok("library_mygo.json"),
		mygoTorrents:    searchingSites(writeFixture(t, items)), downloadPath: ok("download_added.json"),
	}}
}

func openPlanResources(h *harness, message int) {
	h.t.Helper()
	h.tap(alice, message, "1")
	h.tap(alice, message, "第 1 季")
	h.searchTorrents(alice, message, searchResources)
}

func planLibrary(h *harness, held []int) {
	h.t.Helper()
	h.mp.setRoute(libraryShowPath, ok(writeFixture(h.t, map[string][]int{"1": held})))
}

func planTotal(h *harness, total int) {
	h.t.Helper()
	h.mp.setRoute(seasonsPath, ok(writeFixture(h.t, []map[string]any{{
		"season_number": 1, "episode_count": total, "name": "第 1 季",
	}})))
}

func planNoButton(h *harness, message int, label string) {
	h.t.Helper()
	if _, found := findButton(mustMessage(h, message).rows, label); found {
		h.t.Fatalf("message %d unexpectedly offers %q", message, label)
	}
}

func planCalls(h *harness, counts planCallCounts) {
	h.t.Helper()
	for path, want := range map[string]int{mygoTorrents: counts.searches, downloadPath: counts.downloads} {
		if got := strings.Count(h.tr.String(), "-> MoviePilot "+path); got != want {
			h.t.Errorf("%s calls: got %d, want %d", path, got, want)
		}
	}
}

func planExcludedCandidates(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	changes := []struct {
		name, section, key string
		value              any
	}{
		{"WrongID", "media_info", "media_id", "999999"},
		{"WrongSource", "media_info", "media_source", "douban"},
		{"WrongKind", "media_info", "type", "电影"},
		{"WrongSeason", "meta_info", "begin_season", 2},
		{"UnknownSeason", "meta_info", "begin_season", nil},
		{"MultipleSeasons", "meta_info", "end_season", 2},
		{"MetaGroup", "meta_info", "episode_group", "synthetic-group"},
		{"MediaGroup", "media_info", "episode_group", "synthetic-group"},
	}
	for _, change := range changes {
		item := planRelease(t, planReleaseSpec{name: change.name, episodes: []int{2, 3, 4, 5}, site: "站点甲", size: planGiB})
		item[change.section].(map[string]any)[change.key] = change.value
		out = append(out, item)
	}
	return out
}

func planCoverageCandidates(t *testing.T) []map[string]any {
	t.Helper()
	items := []map[string]any{
		planRelease(t, planReleaseSpec{name: "CoverA", episodes: []int{1, 3, 4}, site: "站点甲", size: 3 * planGiB, seeders: 10}),
		planRelease(t, planReleaseSpec{name: "CoverB", episodes: []int{3, 5}, site: "站点甲", hr: true}),
		planRelease(t, planReleaseSpec{name: "LargerTie", episodes: []int{1, 3, 4}, site: "站点甲", size: 5 * planGiB, seeders: 900}),
		planRelease(t, planReleaseSpec{name: "UnknownPack", site: "站点甲", size: planGiB}),
		planRelease(t, planReleaseSpec{name: "OutOfBounds", episodes: []int{2, 14}, site: "站点甲", size: planGiB}),
	}
	unsure := planRelease(t, planReleaseSpec{name: "UnsurePack", episodes: []int{2, 3, 4, 5}, site: "站点甲", size: planGiB})
	unsure["torrent_info"].(map[string]any)["title"] = "MyGO.S01.1080p-UnsurePack"
	items = append(items, unsure)
	return append(items, planExcludedCandidates(t)...)
}
