package e2e

import "testing"

// Feature banners replace the home image in the same message; returning
// from a chart, resource or task restores the feature's image, not its poster.
// Each banner is uploaded once and reused by file_id afterwards.
func TestHomeFeaturesShowTheirOwnBanners(t *testing.T) {
	routes := taskRoutes()
	routes[subsPath], routes[latestPath] = ok("subscriptions.json"), ok("latest.json")
	routes[trendingPath], routes[showResources] = ok("chart_trending.json"), ok("resources_show.json")
	h := start(t, scenario{routes: routes})
	h.say(alice, alice, "/start")
	h.tap(alice, 1, "发现")
	h.tap(alice, 1, "TMDB 流行趋势")
	h.tap(alice, 1, "返回榜单")
	h.tap(alice, 1, "首页")
	h.tap(alice, 1, "订阅")
	h.tap(alice, 1, "1")
	h.tap(alice, 1, "返回")
	h.tap(alice, 1, "首页")
	h.tap(alice, 1, "最新入库")
	h.tap(alice, 1, "首页")
	h.tap(alice, 1, "任务进度")
	h.tap(alice, 1, mygoTask)
	h.tap(alice, 1, stopRefresh)
	h.tap(alice, 1, "返回任务列表")
	h.tap(alice, 1, "关闭")
	h.tr.verify(t)
}
