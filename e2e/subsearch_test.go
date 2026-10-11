package e2e

import (
	"strings"
	"testing"
)

const subSearchMessage = 2

func searchableMovie() map[string]any {
	return map[string]any{
		"id": 1, "name": "无海报影片", "type": "电影", "media_source": "themoviedb", "media_id": "42",
		"state": "R", "date": "2026-10-05 03:00:00",
	}
}

func subscriptionSearchHarness(t *testing.T) *harness {
	t.Helper()
	h := start(t, receiptScenario(t))
	h.say(alice, alice, "无海报影片")
	h.tap(alice, 1, "确认订阅")
	h.mp.setRoute(subsPath, ok(writeFixture(t, []map[string]any{searchableMovie()})))
	h.say(alice, alice, "/subscribe")
	h.tap(alice, subSearchMessage, "1")
	return h
}

func beginSubscriptionSearch(h *harness) {
	h.t.Helper()
	h.tap(alice, subSearchMessage, "立即搜索")
	h.shows(subSearchMessage, "访问站点")
	h.shows(subSearchMessage, "自动下载")
	h.tap(alice, subSearchMessage, "确认搜索")
}

func assertSubscriptionSearchCalls(h *harness, posts, cancels int) {
	h.t.Helper()
	if got := h.mp.searchCalls(subSearchPost); got != posts {
		h.t.Fatalf("subscription-search POST count = %d, want %d", got, posts)
	}
	if got := h.mp.searchCalls(subBatchCancel); got != cancels {
		h.t.Fatalf("batch-cancel PUT count = %d, want %d", got, cancels)
	}
}

func assertNoSearchSecrets(h *harness) {
	h.t.Helper()
	for _, secret := range []string{"fake-passkey", "hidden-search-error", "private-site", "/srv/private"} {
		if strings.Contains(h.tr.String(), secret) {
			h.t.Errorf("subscription search exposed %q", secret)
		}
	}
}

func TestSubscriptionSearchAfterEditingTracksOneBatch(t *testing.T) {
	h := subscriptionEditor(t)
	chooseEditValue(h, "分辨率", "1")
	h.tap(alice, editMessage, "查看修改")
	h.tap(alice, editMessage, "确认保存")
	assertSubscriptionSearchCalls(h, 0, 0)
	h.tap(alice, editMessage, "刷新")
	h.tap(alice, editMessage, "立即搜索")
	h.tap(alice, editMessage, "返回")
	assertSubscriptionSearchCalls(h, 0, 0)
	h.tap(alice, editMessage, "立即搜索")
	h.shows(editMessage, "访问站点")
	h.shows(editMessage, "自动下载")
	confirm, _ := findButton(mustMessage(h, editMessage).rows, "确认搜索")
	h.tap(alice, editMessage, "确认搜索")
	h.shows(editMessage, "排队中")
	h.tapData(alice, editMessage, confirm)
	h.tap(alice, editMessage, "停止刷新")
	h.mp.setSearchBatch(firstSubBatch, map[string]any{"state": "running", "phase": "waiting_site_budget"})
	h.tap(alice, editMessage, "继续刷新")
	h.shows(editMessage, "等待站点额度")
	h.tap(alice, editMessage, "停止刷新")
	h.mp.setSearchBatch(firstSubBatch, map[string]any{
		"state": "completed", "phase": "completed", "processed_count": 1, "finished_count": 1,
		"current_subscription_id": nil, "can_cancel": false,
	})
	h.tap(alice, editMessage, "刷新")
	h.shows(editMessage, "搜索结束")
	h.shows(editMessage, "不代表")
	h.tap(alice, editMessage, "返回")
	h.shows(editMessage, "订阅详情")
	h.tap(alice, editMessage, "查看本次搜索")
	h.shows(editMessage, "搜索结束")
	h.tap(alice, editMessage, "关闭")
	assertSubscriptionSearchCalls(h, 1, 0)
	assertNoSearchSecrets(h)
	h.tr.verify(t)
}
