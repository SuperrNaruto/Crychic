package e2e

import (
	"strings"
	"testing"
)

// A failed card edit leaves the list visible. Pressing its number again
// resends the already generated card, without rereading or adding history.
func TestRepeatedPickRecoversUndeliveredCard(t *testing.T) {
	const filmID = "42"
	const filmDetails = "GET /api/v1/media/" + filmID
	media := map[string]any{
		"media_source": "themoviedb", "media_id": filmID, "type": "电影", "title": "无海报影片",
	}
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok(writeFixture(t, []map[string]any{media, {
			"media_source": "themoviedb", "media_id": "43", "type": "电影", "title": "另一部影片",
		}})),
		filmDetails:                             ok(writeFixture(t, media)),
		"GET /api/v1/subscribe/media/" + filmID: ok("subscription_none.json"),
	}})
	h.say(alice, alice, "无海报影片")
	failed := h.tg.failNextEdit(1)
	h.tap(alice, 1, "1")
	h.wait(failed, "the rejected card edit")
	h.shows(1, "帮你找到这些")
	h.tap(alice, 1, "1")
	h.shows(1, "要订阅《无海报影片》")
	if strings.Count(h.tr.String(), filmDetails+"?") != 1 {
		t.Fatal("retrying a failed card delivery repeated its backend lookup")
	}
	h.tap(alice, 1, "返回")
	h.shows(1, "帮你找到这些")
	h.tr.verify(t)
}
