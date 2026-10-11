package e2e

import (
	"net/http"
	"strings"
	"testing"
)

func TestSubscriptionSearchCancellationIsOnlyARequest(t *testing.T) {
	for _, state := range []string{"queued", "running"} {
		t.Run(state, func(t *testing.T) {
			h := subscriptionSearchHarness(t)
			beginSubscriptionSearch(h)
			h.tap(alice, subSearchMessage, "停止刷新")
			h.mp.setSearchBatch(firstSubBatch, map[string]any{"state": state, "phase": state})
			h.tap(alice, subSearchMessage, "刷新")
			h.tap(alice, subSearchMessage, "停止本次搜索")
			h.shows(subSearchMessage, "已提交的下载和订阅都会保留")
			h.tap(alice, subSearchMessage, "返回")
			assertSubscriptionSearchCalls(h, 1, 0)
			h.tap(alice, subSearchMessage, "停止本次搜索")
			confirm, _ := findButton(mustMessage(h, subSearchMessage).rows, "确认停止搜索")
			h.tap(alice, subSearchMessage, "确认停止搜索")
			h.shows(subSearchMessage, "停止请求已提交")
			h.tapData(alice, subSearchMessage, confirm)
			h.tap(alice, subSearchMessage, "刷新")
			if state == "running" {
				h.shows(subSearchMessage, "正在停止")
				completeCancelledSearch(h)
			}
			h.shows(subSearchMessage, "<td>批次状态</td><td>已停止</td>")
			assertNoBatchStop(h)
			h.tap(alice, subSearchMessage, "关闭")
			assertSubscriptionSearchCalls(h, 1, 1)
			if strings.Contains(h.tr.String(), "-> MoviePilot "+downloadPath) || strings.Contains(h.tr.String(), "-> MoviePilot PUT /api/v1/subscribe/status/") {
				t.Error("batch cancellation controlled the downloader or subscription scheduling")
			}
			h.tr.verify(t)
		})
	}
}

func completeCancelledSearch(h *harness) {
	h.t.Helper()
	// MoviePilot can finish a task after download submission while the batch
	// settles as cancelled. Count and state are separate facts, not a rollback.
	h.mp.setSearchBatch(firstSubBatch, map[string]any{
		"state": "cancelled", "phase": "cancelled", "finished_count": 1,
		"processed_count": 1, "current_subscription_id": nil,
		"error": "fake-passkey /srv/private download already submitted",
	})
	h.tap(alice, subSearchMessage, "刷新")
	h.shows(subSearchMessage, "已提交的下载")
	assertNoSearchSecrets(h)
}

func TestSubscriptionSearchUncertainStopOnlyReadsAgain(t *testing.T) {
	cases := []struct {
		name     string
		response subscriptionSearchResponse
		want     string
	}{
		{"lost", subscriptionSearchResponse{lost: true}, "停止结果待核对"},
		{"timeout", subscriptionSearchResponse{status: http.StatusRequestTimeout, body: `{"message":"fake-passkey"}`}, "停止结果待核对"},
		{"missing_success", subscriptionSearchResponse{body: `{}`}, "停止结果待核对"},
		{"refused", subscriptionSearchResponse{status: http.StatusOK, body: `{"success":false,"message":"fake-passkey"}`}, "未能提交停止请求"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := subscriptionSearchHarness(t)
			beginSubscriptionSearch(h)
			h.tap(alice, subSearchMessage, "停止本次搜索")
			confirm, _ := findButton(mustMessage(h, subSearchMessage).rows, "确认停止搜索")
			h.mp.searchResponse(subBatchCancel, tc.response)
			h.tap(alice, subSearchMessage, "确认停止搜索")
			h.shows(subSearchMessage, tc.want)
			h.tapData(alice, subSearchMessage, confirm)
			h.tap(alice, subSearchMessage, "刷新")
			assertNoBatchStop(h)
			h.tap(alice, subSearchMessage, "返回")
			h.tap(alice, subSearchMessage, "查看本次搜索")
			assertNoBatchStop(h)
			h.tap(alice, subSearchMessage, "关闭")
			assertSubscriptionSearchCalls(h, 1, 1)
			assertNoSearchSecrets(h)
			h.tr.verify(t)
		})
	}
}
