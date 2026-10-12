package e2e

import (
	"testing"
	"time"
)

const (
	doubanAbyssID = "36576514"
	// the chart posters of 深渊无间 (page one) and 流人 第六季 (page two),
	// and the poster of the Douban search result for 深渊无间
	abyssPoster   = "/view/photo/m_ratio_poster/public/p2931762705.webp"
	liurenPoster  = "/view/photo/m_ratio_poster/public/p2935061677.webp"
	signedPoster  = "/view/photo/l_ratio_poster/public/p2931762705.webp"
	restartBudget = 3 * time.Second
)

// A Douban chart page's posters download in the background as soon as it
// is shown; picking an entry MoviePilot returns as the same Douban media
// shows its card with the chart's poster, already downloaded, so the poster
// is fetched once. Stopping the bot while a later page's poster is still
// downloading ends the download rather than waiting out its budget.
func TestChartPostersDownloadAheadAndCardKeepsThePoster(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		doubanTVPath:                         ok("chart_douban_tv.json"),
		searchPath:                           ok("search_abyss_douban.json"),
		"GET /api/v1/media/" + doubanAbyssID: ok("detail_abyss.json"),
		seasonsPath:                          ok("seasons_abyss.json"),
		"GET /api/v1/subscribe/media/" + doubanAbyssID: ok("subscription_none.json"),
	}})
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "豆瓣热门剧集")
	h.waitImage(abyssPoster)
	h.tap(alice, 1, "1")
	h.shows(1, "要订阅《深渊无间》第 1 季")
	if got := h.img.downloads(abyssPoster); got != 1 {
		t.Errorf("chart poster downloaded %d times, want 1", got)
	}
	if got := h.img.downloads(signedPoster); got != 0 {
		t.Errorf("search result's own poster downloaded %d times, want 0", got)
	}
	h.tap(alice, 1, "返回")
	h.shows(1, "第 1/2 页")
	release := h.img.hold(liurenPoster)
	defer release()
	h.tap(alice, 1, "下一页 ›")
	h.waitImage(liurenPoster)
	began := time.Now()
	h.restart()
	if took := time.Since(began); took > restartBudget {
		t.Errorf("stopping with a poster downloading took %v, want under %v", took, restartBudget)
	}
	h.tr.verify(t)
}

// waitImage returns once the fake image host has received a request for path.
func (h *harness) waitImage(path string) {
	h.t.Helper()
	select {
	case <-h.img.firstRequest(path):
	case <-time.After(actionTimeout):
		h.t.Fatalf("poster %s was never requested", path)
	}
}
