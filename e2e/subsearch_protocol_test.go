package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestSubscriptionSearchWriteFailuresNeverReplay(t *testing.T) {
	cases := []struct {
		name     string
		response subscriptionSearchResponse
		want     string
	}{
		{"unauthorized", subscriptionSearchResponse{status: http.StatusUnauthorized, body: `{"message":"fake-passkey"}`}, "API Key"},
		{"forbidden", subscriptionSearchResponse{status: http.StatusForbidden, body: `{"message":"fake-passkey"}`}, "API Key"},
		{"refused", subscriptionSearchResponse{status: http.StatusOK, body: `{"success":false,"message":"fake-passkey /srv/private"}`}, "未能提交搜索"},
		{"invalid", subscriptionSearchResponse{status: http.StatusBadRequest, body: `{"message":"fake-passkey"}`}, "未能提交搜索"},
		{"timeout", subscriptionSearchResponse{status: http.StatusRequestTimeout, body: `{"message":"fake-passkey"}`}, "搜索结果待核对"},
		{"missing_success", subscriptionSearchResponse{body: `{}`}, "搜索结果待核对"},
		{"lost", subscriptionSearchResponse{lost: true}, "搜索结果待核对"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := subscriptionSearchHarness(t)
			h.tap(alice, subSearchMessage, "立即搜索")
			confirm, _ := findButton(mustMessage(h, subSearchMessage).rows, "确认搜索")
			h.mp.searchResponse(subSearchPost, tc.response)
			h.tap(alice, subSearchMessage, "确认搜索")
			h.shows(subSearchMessage, tc.want)
			h.tapData(alice, subSearchMessage, confirm)
			h.tap(alice, subSearchMessage, "返回")
			h.tap(alice, subSearchMessage, "查看本次搜索")
			h.shows(subSearchMessage, tc.want)
			h.tap(alice, subSearchMessage, "关闭")
			assertSubscriptionSearchCalls(h, 1, 0)
			if h.mp.searchCalls(subBatchRead) != 0 {
				t.Error("an unknown or refused write was adopted as an owned batch")
			}
			assertNoSearchSecrets(h)
			h.tr.verify(t)
		})
	}
}

func TestSubscriptionSearchNeverAdoptsAmbiguousSubmission(t *testing.T) {
	cases := []struct {
		name   string
		fields map[string]any
	}{
		{"missing_count", map[string]any{"queued_count": nil}}, {"not_single", map[string]any{"single": false}},
		{"extra_target", map[string]any{"target_count": 2}}, {"extra_batch", map[string]any{"batch_ids": []string{firstSubBatch, "foreign-batch"}}},
		{"different_id", map[string]any{"batch_id": "foreign-batch"}},
		{"unsafe_id", map[string]any{"batch_id": "../foreign-batch", "batch_ids": []string{"../foreign-batch"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := subscriptionSearchHarness(t)
			submission := searchSubmission(firstSubBatch, true)
			for field, value := range tc.fields {
				submission[field] = value
			}
			raw, _ := json.Marshal(searchEnvelope(true, "", submission))
			h.mp.searchResponse(subSearchPost, subscriptionSearchResponse{body: string(raw)})
			beginSubscriptionSearch(h)
			h.shows(subSearchMessage, "搜索结果待核对")
			if h.mp.searchCalls(subBatchRead) != 0 {
				t.Error("ambiguous search response enabled batch tracking")
			}
			h.tap(alice, subSearchMessage, "关闭")
			assertSubscriptionSearchCalls(h, 1, 0)
			h.tr.verify(t)
		})
	}
}

func TestSubscriptionSearchDoesNotTakeOverExistingBatch(t *testing.T) {
	h := subscriptionSearchHarness(t)
	const foreign = "foreign-batch"
	h.mp.setSearchBatch(foreign, map[string]any{"source": "fallback", "total_count": 2, "state": "cancelling", "can_cancel": false})
	beginSubscriptionSearch(h)
	h.shows(subSearchMessage, "已有搜索正在处理")
	h.shows(subSearchMessage, "查看和停止它请到 MoviePilot")
	if h.mp.searchCalls("GET "+subBatchPrefix+foreign) != 0 {
		t.Error("the bot inspected an existing shared batch as its own")
	}
	assertNoBatchStop(h)
	h.tap(alice, subSearchMessage, "返回")
	h.tap(alice, subSearchMessage, "查看本次搜索")
	h.tap(alice, subSearchMessage, "关闭")
	assertSubscriptionSearchCalls(h, 1, 0)
	h.tr.verify(t)
}

func assertNoBatchStop(h *harness) {
	h.t.Helper()
	if _, found := findButton(mustMessage(h, subSearchMessage).rows, "停止本次搜索"); found {
		h.t.Fatal("unverified or consumed batch still offers cancellation")
	}
}

func TestSubscriptionSearchBatchReadsKeepUncertainty(t *testing.T) {
	h := subscriptionSearchHarness(t)
	beginSubscriptionSearch(h)
	h.tap(alice, subSearchMessage, "停止刷新")
	for _, tc := range invalidSearchBatchFields() {
		batch := newSubscriptionBatch(firstSubBatch, 1)
		for k, v := range tc.fields {
			batch[k] = v
		}
		raw, _ := json.Marshal(searchEnvelope(true, "", batch))
		h.mp.searchResponse(subBatchRead, subscriptionSearchResponse{body: string(raw)})
		h.tap(alice, subSearchMessage, "刷新")
		h.shows(subSearchMessage, tc.want)
		assertNoBatchStop(h)
	}
	for _, body := range []string{`{"success":true,"data":null}`, `{"success":true,"data":{}}`, `{"success":false,"message":"fake-passkey"}`} {
		h.mp.searchResponse(subBatchRead, subscriptionSearchResponse{body: body})
		h.tap(alice, subSearchMessage, "刷新")
		h.shows(subSearchMessage, "暂时无法")
		assertNoBatchStop(h)
	}
	h.mp.searchResponse(subBatchRead, subscriptionSearchResponse{status: http.StatusServiceUnavailable, body: `{"message":"fake-passkey"}`})
	h.tap(alice, subSearchMessage, "刷新")
	assertNoBatchStop(h)
	h.tap(alice, subSearchMessage, "刷新")
	h.shows(subSearchMessage, "排队中")
	h.tap(alice, subSearchMessage, "停止本次搜索")
	h.tap(alice, subSearchMessage, "返回")
	h.tap(alice, subSearchMessage, "关闭")
	assertSubscriptionSearchCalls(h, 1, 0)
	assertNoSearchSecrets(h)
	h.tr.verify(t)
}

type batchReadCase struct {
	fields map[string]any
	want   string
}

func invalidSearchBatchFields() []batchReadCase {
	return []batchReadCase{
		{map[string]any{"batch_id": "foreign-batch"}, "范围"}, {map[string]any{"source": "fallback"}, "范围"},
		{map[string]any{"total_count": 2}, "范围"}, {map[string]any{"current_subscription_id": 2}, "范围"},
		{map[string]any{"current_subscription_id": nil}, "范围"}, {map[string]any{"created_at": "2026-10-05T04:01:00Z"}, "范围"},
		{map[string]any{"processed_count": -1}, "暂时无法"}, {map[string]any{"processed_count": 1, "finished_count": 0}, "暂时无法"},
		{map[string]any{"finished_count": nil}, "暂时无法"},
		{map[string]any{"state": "completed", "phase": "completed", "current_subscription_id": nil}, "暂时无法"},
		{map[string]any{"phase": "hidden-search-error"}, "未知"},
		{map[string]any{"can_cancel": nil}, "排队中"},
		{map[string]any{"state": "private-site", "phase": "hidden-search-error", "error": "fake-passkey /srv/private"}, "未知"},
	}
}

func TestSubscriptionSearchRedirectsNeverLeaveOrigin(t *testing.T) {
	for _, operation := range []string{"submit", "read", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			h := subscriptionSearchHarness(t)
			var reached atomic.Bool
			other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached.Store(true)
				writeEnvelope(w, http.StatusOK, `{"success":true,"data":null}`)
			}))
			defer other.Close()
			redirectSubscriptionOperation(h, operation, other.URL)
			if reached.Load() {
				t.Error("subscription execution followed a redirect with API credentials")
			}
			if h.mp.searchCalls(subSearchPost) != 1 {
				t.Error("redirect replayed the search submission")
			}
			h.tap(alice, subSearchMessage, "关闭")
			h.tr.verify(t)
		})
	}
}

func redirectSubscriptionOperation(h *harness, operation, origin string) {
	h.t.Helper()
	response := subscriptionSearchResponse{location: origin + "/redirected"}
	switch operation {
	case "submit":
		h.mp.searchResponse(subSearchPost, response)
		beginSubscriptionSearch(h)
		h.shows(subSearchMessage, "搜索结果待核对")
	case "read":
		h.mp.searchResponse(subBatchRead, response)
		beginSubscriptionSearch(h)
		h.shows(subSearchMessage, "暂时无法")
	case "cancel":
		beginSubscriptionSearch(h)
		h.tap(alice, subSearchMessage, "停止本次搜索")
		h.mp.searchResponse(subBatchCancel, response)
		h.tap(alice, subSearchMessage, "确认停止搜索")
		h.shows(subSearchMessage, "停止结果待核对")
		assertSubscriptionSearchCalls(h, 1, 1)
	}
}
