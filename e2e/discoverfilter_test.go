package e2e

import (
	"strings"
	"testing"
)

const (
	tmdbTVs        = "GET /api/v1/discover/tmdb_tvs"
	tmdbMovies     = "GET /api/v1/discover/tmdb_movies"
	findLabel      = "开始找片"
	moreLabel      = "更多结果"
	conditionsNote = "条件：电视剧 · 动画 · 日语 · ≥100 · 近 1 年 · 评分最高"
)

// Someone picks conditions on one panel: each field opens its options in
// place and closes on a choice. The search sends them to TMDB as lower
// bounds; 更多结果 reads the next page, a result already listed keeps its
// number, and 返回 shows the panel with its conditions. Switching kind
// clears the genre, whose ids differ by kind, and a search finding nothing
// only flashes a notice.
func TestDiscoverByConditions(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		tmdbTVs:                    ok("discover_anime_p1.json"),
		discoverPage(tmdbTVs, "2"): ok("discover_anime_p2.json"),
		tmdbMovies:                 ok("empty.json"),
	}})
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "条件找片")
	h.tap(alice, 1, "电视剧")
	for _, pick := range [][2]string{{"题材", "动画"}, {"语言", "日语"}, {"评分人数", "≥100"}, {"上映", "近 1 年"}, {"排序", "评分最高"}} {
		h.tap(alice, 1, pick[0])
		h.shows(1, "选一个「"+pick[0]+"」")
		h.tap(alice, 1, pick[1])
	}
	h.shows(1, "<td>题材</td><td>动画</td>")
	h.tap(alice, 1, findLabel)
	h.shows(1, conditionsNote)
	h.tap(alice, 1, moreLabel)
	h.shows(1, "排球少年")
	if text := mustMessage(h, 1).text; strings.Count(text, "乱马½") != 1 || !strings.Contains(text, `value="21"`) {
		t.Errorf("the next page relisted a result or renumbered the list:\n%s", text)
	}
	h.tap(alice, 1, "返回")
	h.shows(1, "<td>语言</td><td>日语</td>")
	h.tap(alice, 1, "电影")
	h.shows(1, "<td>题材</td><td>不限</td>")
	h.tap(alice, 1, findLabel)
	h.shows(1, "<td>类型</td><td>电影</td>")
	h.tr.verify(t)
}
