package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Details are read-only, join progress by media identity and season, and
// keep library visibility separate from download/transfer completion.
func TestSubscriptionDetails(t *testing.T) {
	h := start(t, scenario{routes: subscriptionDetailRoutes()})
	h.say(alice, group, "/subscribe")
	h.tap(bob, 1, "1")
	h.shows(1, "订阅清单")
	h.tap(alice, 1, "1")
	h.shows(1, "订阅详情")
	h.shows(1, "等待站点额度")
	h.shows(1, "2026-10-05 13:30")
	h.shows(1, "37%")
	h.shows(1, "整理失败")
	h.shows(1, "季或集数不明")
	subs := detailSubscriptions(h)
	subs[0]["state"] = "S"
	subs[0]["resolution"] = "2160p"
	subs[0]["execution_status"] = map[string]any{"state": "failed", "phase": "failed", "error": "request to https://tracker.invalid/?passkey=do-not-show failed"}
	h.mp.setRoute(subsPath, ok(writeFixture(t, subs)))
	h.mp.setRoute(downloadsPath, ok("downloads_none.json"))
	h.mp.setRoute(queuePath, ok("queue_none.json"))
	h.tap(alice, 1, "刷新")
	h.shows(1, "2160p")
	h.shows(1, "执行失败")
	h.tap(alice, 1, "返回")
	h.shows(1, "订阅清单")
	h.mp.setRoute(downloadsPath, ok("downloads_subscription.json"))
	h.mp.setRoute(queuePath, ok("queue_subscription.json"))
	h.tap(alice, 1, "电影")
	h.tap(alice, 1, "1")
	h.shows(1, "《沙丘》")
	h.shows(1, "已入库")
	h.shows(1, "暂无执行记录")
	h.shows(1, "已暂停 62%")
	if strings.Contains(mustMessage(h, 1).text, "整理失败") {
		t.Fatal("a TV special with the same numeric ID must not supply the movie's progress")
	}
	for _, private := range []string{"tracker.invalid", "do-not-show", "/srv/private", "private-release", "91%", "95%", "98%"} {
		if strings.Contains(h.tr.String(), private) {
			t.Fatalf("details disclosed or misattributed %q", private)
		}
	}
	h.tr.verify(t)
}

// Failed reads leave navigation usable. Missing enrichment is unknown,
// not proof of absence; a refresh retries it without adding a back step.
func TestSubscriptionDetailReadsRecover(t *testing.T) {
	routes := subscriptionDetailRoutes()
	h := start(t, scenario{routes: routes})
	h.say(alice, alice, "/subscribe")
	down := route{status: http.StatusServiceUnavailable, fixture: "server_error.json"}
	h.mp.setRoute(subsPath, down)
	h.tap(alice, 1, "1")
	h.shows(1, "订阅清单")
	h.mp.setRoute(subsPath, routes[subsPath])
	for _, key := range []string{libraryShowPath, downloadsPath, queuePath} {
		h.mp.setRoute(key, down)
	}
	h.tap(alice, 1, "1")
	h.shows(1, "订阅详情")
	h.shows(1, "媒体库暂时无法确认")
	h.shows(1, "下载进度暂时无法确认")
	h.shows(1, "整理进度暂时无法确认")
	for _, key := range []string{libraryShowPath, downloadsPath, queuePath} {
		h.mp.setRoute(key, routes[key])
	}
	h.tap(alice, 1, "刷新")
	h.shows(1, "37%")
	h.tap(alice, 1, "返回")
	h.shows(1, "订阅清单")
	h.tap(alice, 1, "2")
	h.shows(1, "总集数暂未确定")
	h.shows(1, "暂无执行记录")
	h.tr.verify(t)
}

// A subscription can end while its list/detail remains open. Refreshing
// cannot resurrect it, and an expired detail leads back to /subscribe.
func TestSubscriptionDetailEnded(t *testing.T) {
	h := start(t, scenario{routes: subscriptionDetailRoutes()})
	h.say(alice, alice, "/subscribe")
	h.tap(alice, 1, "1")
	refresh, _ := findButton(mustMessage(h, 1).rows, "刷新")
	h.mp.setRoute(subsPath, ok("subscriptions_none.json"))
	h.tap(alice, 1, "刷新")
	h.shows(1, "已经结束或被取消")
	h.tap(alice, 1, "返回")
	h.shows(1, "现在还没有订阅")
	const beyondSession = 11 * time.Minute
	h.advance(beyondSession)
	h.tapData(alice, 1, refresh)
	h.shows(1, "/subscribe")
	h.tr.verify(t)
}

func subscriptionDetailRoutes() map[string]route {
	return map[string]route{
		subsPath:         ok("subscriptions_details.json"),
		libraryShowPath:  ok("library_subscription.json"),
		libraryMoviePath: ok("library_movie_held.json"),
		downloadsPath:    ok("downloads_subscription.json"),
		queuePath:        ok("queue_subscription.json"),
	}
}

func detailSubscriptions(h *harness) []map[string]any {
	h.t.Helper()
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(fixture(h.mp, "subscriptions_details.json"), &env); err != nil {
		h.t.Fatal(err)
	}
	return env.Data
}
