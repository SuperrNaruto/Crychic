package e2e

import (
	"net/http"
	"testing"
)

func TestSubscriptionSearchRechecksIdentityAndConditions(t *testing.T) {
	h := subscriptionSearchHarness(t)
	h.tap(alice, subSearchMessage, "立即搜索")
	confirm, _ := findButton(mustMessage(h, subSearchMessage).rows, "确认搜索")
	h.tap(bob, subSearchMessage, "确认搜索")
	for _, change := range []struct {
		field string
		value any
	}{
		{"media_source", "douban"}, {"media_id", "43"}, {"type", "电视剧"},
		{"season", 2}, {"episode_group", "another-group"}, {"date", "2026-10-05 03:01:00"},
	} {
		sub := searchableMovie()
		sub[change.field] = change.value
		h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{sub})))
		h.tap(alice, subSearchMessage, "确认搜索")
		assertSubscriptionSearchCalls(h, 0, 0)
	}
	h.mp.setRoute(subsPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	h.tap(alice, subSearchMessage, "确认搜索")
	assertSubscriptionSearchCalls(h, 0, 0)
	sub := searchableMovie()
	sub["state"], sub["include"] = "S", "hidden-search-error /srv/private"
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{sub})))
	h.tap(alice, subSearchMessage, "确认搜索")
	h.shows(subSearchMessage, "已变化")
	h.shows(subSearchMessage, "已暂停")
	h.shows(subSearchMessage, "不会恢复")
	h.tapData(alice, subSearchMessage, confirm)
	assertSubscriptionSearchCalls(h, 0, 0)
	h.tap(alice, subSearchMessage, "确认搜索")
	h.shows(subSearchMessage, "排队中")
	h.tap(alice, subSearchMessage, "关闭")
	assertSubscriptionSearchCalls(h, 1, 0)
	assertNoSearchSecrets(h)
	checkSearchWithoutCreationDate(h)
	h.tr.verify(t)
}

func checkSearchWithoutCreationDate(h *harness) {
	h.t.Helper()
	sub := searchableMovie()
	delete(sub, "date")
	h.mp.setRoute(subsPath, ok(writeFixture(h.t, []map[string]any{sub})))
	h.say(alice, alice, "/subscribe")
	const missingDateMessage = 3
	h.tap(alice, missingDateMessage, "1")
	h.tap(alice, missingDateMessage, "立即搜索")
	h.shows(missingDateMessage, "订阅详情")
	assertSubscriptionSearchCalls(h, 1, 0)
}

func TestSubscriptionSearchRevocationRemovesControl(t *testing.T) {
	h := subscriptionSearchHarness(t)
	beginSubscriptionSearch(h)
	h.tap(alice, subSearchMessage, "停止刷新")
	h.tap(alice, subSearchMessage, "停止本次搜索")
	h.mp.setRoute(deleteFirst, ok("subscribe_deleted.json"))
	h.say(alice, alice, "/subscribe")
	const anotherList = 3
	h.tap(alice, anotherList, "取消订阅")
	h.tap(alice, anotherList, "1")
	h.tap(alice, anotherList, "确认取消")
	h.tap(alice, subSearchMessage, "确认停止搜索")
	assertSubscriptionSearchCalls(h, 1, 0)
	h.tap(alice, subSearchMessage, "返回")
	h.shows(subSearchMessage, "订阅搜索")
	h.tap(alice, subSearchMessage, "关闭")
	h.tr.verify(t)
}

func TestSubscriptionSearchStopRechecksScope(t *testing.T) {
	h := subscriptionSearchHarness(t)
	beginSubscriptionSearch(h)
	h.tap(alice, subSearchMessage, "停止刷新")
	h.tap(alice, subSearchMessage, "停止本次搜索")
	h.mp.setSearchBatch(firstSubBatch, map[string]any{"total_count": 2})
	h.tap(alice, subSearchMessage, "确认停止搜索")
	h.shows(subSearchMessage, "范围")
	assertSubscriptionSearchCalls(h, 1, 0)
	h.mp.setSearchBatch(firstSubBatch, map[string]any{"total_count": 1})
	h.tap(alice, subSearchMessage, "刷新")
	h.tap(alice, subSearchMessage, "停止本次搜索")
	sub := searchableMovie()
	sub["date"] = "2026-10-05 03:01:00"
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{sub})))
	h.tap(alice, subSearchMessage, "确认停止搜索")
	assertSubscriptionSearchCalls(h, 1, 0)
	h.tap(alice, subSearchMessage, "返回")
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{searchableMovie()})))
	h.tap(alice, subSearchMessage, "停止本次搜索")
	h.mp.setSearchBatch(firstSubBatch, map[string]any{
		"state": "completed", "phase": "completed", "finished_count": 1, "processed_count": 1,
		"current_subscription_id": nil, "can_cancel": false,
	})
	h.tap(alice, subSearchMessage, "确认停止搜索")
	h.shows(subSearchMessage, "搜索结束")
	assertSubscriptionSearchCalls(h, 1, 0)
	h.tap(alice, subSearchMessage, "关闭")
	h.tr.verify(t)
}
