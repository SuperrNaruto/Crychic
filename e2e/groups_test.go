package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	kimetsuDetails      = "GET /api/v1/media/85937"
	kimetsuGroups       = "GET /api/v1/media/groups/85937"
	kimetsuWikiSeasons  = "GET /api/v1/media/group/seasons/66c6dcd6cd3639e51ad7ec9f"
	kimetsuWikiGroup    = "4" // Wikpedia / News once the empty Crunchyroll group is left out
	kimetsuGroupSeason2 = "第 2 季"
)

// Someone subscribes a season numbered by an episode group: groups mapping
// nothing are left out, the seasons and episode counts are the group's, a
// standard season 2 subscription is not taken for the group's, facts kept
// in standard seasons (library, next episode, resources, upgrade) are not
// offered, and the subscription carries the group.
func TestSubscribeByEpisodeGroup(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok("search_kimetsu.json"), kimetsuDetails: ok("detail_kimetsu.json"),
		seasonsPath: ok("seasons_kimetsu.json"), kimetsuGroups: ok("groups_kimetsu.json"),
		kimetsuWikiSeasons: ok("group_seasons_kimetsu.json"), subsPath: ok(standardKimetsuSeason2(t)),
		subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 鬼灭之刃")
	h.tap(alice, 1, "剧集组…")
	if strings.Contains(mustMessage(h, 1).text, "Crunchyroll</b><br>") {
		t.Error("an episode group mapping no episodes is offered")
	}
	h.tap(alice, 1, kimetsuWikiGroup)
	h.shows(1, "按剧集组「Wikpedia / News」的编排")
	h.shows(1, "<td>18 集</td>")
	h.tap(alice, 1, kimetsuGroupSeason2)
	h.shows(1, "剧集组「Wikpedia / News」")
	for _, label := range []string{"搜索资源", "洗版订阅", "只追新集"} {
		if _, shown := findButton(mustMessage(h, 1).rows, label); shown {
			t.Errorf("a group season offers %s, which works in standard seasons", label)
		}
	}
	h.tap(alice, 1, "从第 1 集开始")
	h.shows(1, "帮你订好《鬼灭之刃》第 2 季（剧集组「Wikpedia / News」）")
	h.tr.verify(t)
}

// standardKimetsuSeason2 is the subscription list with 鬼灭之刃's standard
// season 2 subscribed.
func standardKimetsuSeason2(t *testing.T) string {
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
	sub["id"], sub["name"], sub["media_id"], sub["year"], sub["episode_group"] = 9, "鬼灭之刃", "85937", "2019", nil
	return writeFixture(t, env.Data)
}
