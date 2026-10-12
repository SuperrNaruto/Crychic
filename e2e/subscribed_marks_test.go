package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A title subscribed already says so before it is picked: search results
// tag a subscribed movie and the subscribed seasons of a show, the season
// picker gains a 订阅 column (a paused season named as such, an episode
// group's season left out), and a subscription list that cannot be read
// leaves both without marks.
func TestSearchAndSeasonsShowSubscriptions(t *testing.T) {
	subs := []any{
		markedSubscription(t, "438631", "电影", nil, "R", nil),
		markedSubscription(t, "693134", "电影", nil, "S", nil),
		markedSubscription(t, "90228", "电视剧", 1, "R", nil),
		markedSubscription(t, "90228", "电视剧", 2, "S", nil),
		markedSubscription(t, "85937", "电视剧", 1, "R", nil),
		markedSubscription(t, "85937", "电视剧", 2, "S", nil),
		markedSubscription(t, "85937", "电视剧", 3, "R", "66c6dcd6cd3639e51ad7ec9f"),
	}
	h := start(t, scenario{
		routes: map[string]route{
			subsPath: ok(writeFixture(t, subs)), kimetsuDetails: ok("detail_kimetsu.json"),
			seasonsPath: ok("seasons_kimetsu.json"),
		},
		searches: map[string]string{"沙丘": "search_dune.json", "鬼灭之刃": "search_kimetsu.json"},
	})
	h.say(alice, alice, "沙丘")
	h.shows(1, "<mark>已订阅</mark>")
	h.shows(1, "<mark>已订阅第 1 季，已暂停第 2 季</mark>")
	if n := strings.Count(mustMessage(h, 1).text, "<mark>"); n != 3 {
		t.Errorf("results carry %d marks, want 3", n)
	}
	h.say(alice, alice, "鬼灭之刃")
	h.shows(2, "<th>订阅</th>")
	h.shows(2, "<td>已订阅</td>")
	h.shows(2, "<td>已暂停</td>")
	if strings.Count(mustMessage(h, 2).text, "<td>已订阅</td>") != 1 {
		t.Error("an episode group's subscription marks a standard season")
	}
	h.mp.setRoute(subsPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.say(alice, alice, "沙丘")
	if strings.Contains(mustMessage(h, 3).text, "已订阅") {
		t.Error("results are marked though the subscription list failed")
	}
	h.say(alice, alice, "鬼灭之刃")
	if strings.Contains(mustMessage(h, 4).text, "订阅</th>") {
		t.Error("seasons show a subscription column though the list failed")
	}
	h.tr.verify(t)
}

// markedSubscription is a subscription row for a TMDB title, derived from
// the recorded list's first row.
func markedSubscription(t *testing.T, id, kind string, season any, state string, group any) map[string]any {
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
	sub["id"], sub["media_id"], sub["type"], sub["state"], sub["season"] = 100, id, kind, state, season
	sub["episode_group"] = group
	return sub
}
