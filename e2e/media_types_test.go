package e2e

import (
	"strings"
	"testing"
)

// MoviePilot lists music alongside video subscriptions; unsupported kinds
// must not turn into movie cards or count as browseable video subscriptions.
func TestSubscriptionListIgnoresUnsupportedMedia(t *testing.T) {
	music := map[string]any{"id": 70, "name": "不属于影视的专辑", "type": "音乐", "media_source": "musicbrainz", "media_id": "album-one", "state": "R"}
	unknown := map[string]any{"id": 71, "name": "未知分类项目", "type": "未知", "state": "N"}
	movie := map[string]any{"id": 72, "name": "沙丘", "type": "电影", "media_source": "themoviedb", "media_id": "438631", "state": "R"}
	tv := map[string]any{"id": 73, "name": "迷途之子!!!!!", "type": "电视剧", "media_source": "themoviedb", "media_id": mygo.id, "season": 1, "state": "R"}
	h := start(t, scenario{routes: map[string]route{
		subsPath: ok(writeFixture(t, []map[string]any{music, movie, unknown, tv})),
	}})
	h.say(alice, alice, "/subscribe")
	h.shows(1, "电视剧（1）")
	h.tap(alice, 1, "电影")
	if text := mustMessage(h, 1).text; !strings.Contains(text, "电影（1）") || strings.Contains(text, "专辑") || strings.Contains(text, "未知分类") {
		t.Error("unsupported media kinds are listed as movie subscriptions")
	}
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{music, unknown})))
	h.say(alice, alice, "/subscribe")
	h.shows(2, "现在还没有订阅")
	h.tr.verify(t)
}
