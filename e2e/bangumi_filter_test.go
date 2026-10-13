package e2e

import (
	"strings"
	"testing"
)

const (
	satoAnime = "634708"
	satoManga = "312453"
	satoNovel = "296610"
)

// A title search lists the TMDB and Douban matches and only the Bangumi
// anime: the manga is dropped by its subject type, a subject whose type
// cannot be read stays, and a known type is remembered, so searching again
// asks Bangumi only about the one it could not read.
// (search_sato_bangumi.json and the three subject fixtures are synthetic,
// shaped like a live MoviePilot v3.1.4 answer and the Bangumi public API.)
func TestTitleSearchKeepsBangumiAnimeOnly(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{searchPath: ok("search_sato_bangumi.json")}})
	h.bgm.setDown(satoNovel, true)
	h.say(alice, alice, "冷淡的佐藤同学")
	h.shows(1, "这些「冷淡的佐藤同学」啦")
	first := mustMessage(h, 1).text
	if strings.Count(first, `<li value=`) != 4 || strings.Contains(first, "@comic") {
		t.Errorf("want TMDB, Douban, the anime and the unreadable subject; got\n%s", first)
	}
	h.bgm.setDown(satoNovel, false)
	before := strings.Count(h.tr.String(), subjectLine)
	h.say(alice, alice, "冷淡的佐藤同学")
	h.shows(2, "这些「冷淡的佐藤同学」啦")
	if got := strings.Count(h.tr.String(), subjectLine) - before; got != 1 {
		t.Errorf("second search asked Bangumi %d times, want only the unread subject", got)
	}
	if strings.Count(mustMessage(h, 2).text, `<li value=`) != 3 {
		t.Errorf("the novel stayed once its type was known:\n%s", mustMessage(h, 2).text)
	}
	h.tr.verify(t)
}
