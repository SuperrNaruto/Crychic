package e2e

import (
	"strings"
	"testing"
)

// Feature banners replace the home image in the same message; returning
// from a chart, resource or task restores the feature's image, not its poster.
func TestHomeFeaturesShowTheirOwnBanners(t *testing.T) {
	const home = "d620fc3c5f168f03372b7fd77d4572d577b95fa1f410d95e2b60e5799f0d2315"
	const discover = "aec4090e605ebe6e873d06d87e188c608c0948099e47aa633df199dd2c483443"
	const subscriptions = "7b987883d5fc3010605ea37711d6b2eb6184479e790f4096dc79c790b644d27f"
	const latest = "9dde60da15adb73db35b2a37cf6a80def9dbfcb4678f09dea4bdd7bf3dc45518"
	const tasks = "073881ee9990d0b268dc6dd099e569afe5d364e5014f11a6c07d0beaed89248c"
	routes := taskRoutes()
	routes[subsPath], routes[latestPath] = ok("subscriptions.json"), ok("latest.json")
	routes[trendingPath], routes[showResources] = ok("chart_trending.json"), ok("resources_show.json")
	h := start(t, scenario{routes: routes})
	h.say(alice, alice, "/start")
	h.tap(alice, 1, "发现")
	h.showsPhoto(1, discover)
	h.tap(alice, 1, "TMDB 流行趋势")
	h.tap(alice, 1, "返回榜单")
	h.showsPhoto(1, discover)
	h.tap(alice, 1, "首页")
	h.showsPhoto(1, home)
	h.tap(alice, 1, "订阅")
	h.showsPhoto(1, subscriptions)
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "返回")
	h.showsPhoto(1, subscriptions)
	h.tap(alice, 1, "首页")
	h.tap(alice, 1, "最新入库")
	h.showsPhoto(1, latest)
	h.tap(alice, 1, "首页")
	h.showsPhoto(1, home)
	h.tap(alice, 1, "任务进度")
	h.showsPhoto(1, tasks)
	h.tap(alice, 1, mygoTask)
	h.tap(alice, 1, stopRefresh)
	h.tap(alice, 1, "返回任务列表")
	h.showsPhoto(1, tasks)
	h.tap(alice, 1, "关闭")
	h.tr.verify(t)
}

// The fake derives a photo's file_id from the actual uploaded bytes, so
// this checks the supplied JPEG even if the bot later reuses that file_id.
func (h *harness) showsPhoto(msgID int, digest string) {
	h.t.Helper()
	want := `<img src="file-` + digest[:fileIDLength] + `"/>`
	if !strings.Contains(mustMessage(h, msgID).seen(), want) {
		h.t.Fatalf("message %d does not show photo sha256=%s", msgID, digest)
	}
}
