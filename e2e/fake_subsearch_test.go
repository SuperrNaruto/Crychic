package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	subSearchPrefix = "/api/v1/subscribe/search/"
	subBatchPrefix  = "/api/v1/subscribe/execution/batches/"
	subSearchPost   = "POST " + subSearchPrefix + "1"
	firstSubBatch   = "search-batch-1"
	subBatchRead    = "GET " + subBatchPrefix + firstSubBatch
	subBatchCancel  = "PUT " + subBatchPrefix + firstSubBatch + "/cancel"
)

// Synthetic v3.1.1 protocol state. Enqueue can coalesce another batch;
// cancellation only immediately stops queued work, never downloader tasks.
type fakeSubscriptionExecution struct {
	batches   map[string]map[string]any
	responses map[string]subscriptionSearchResponse
	requests  map[string]int
	created   int
}

type subscriptionSearchResponse struct {
	status   int
	body     string
	location string
	lost     bool // commit first, then return an incomplete response
	hold     *heldCall
}

func isSubscriptionBatchRead(r *http.Request) bool {
	return r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, subBatchPrefix)
}

func (f *fakeMoviePilot) searchResponse(key string, response subscriptionSearchResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.execution.responses == nil {
		f.execution.responses = map[string]subscriptionSearchResponse{}
	}
	f.execution.responses[key] = response
}

func (f *fakeMoviePilot) searchCalls(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.execution.requests[key]
}

func (f *fakeMoviePilot) setSearchBatch(id string, fields map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.execution.batches == nil {
		f.execution.batches = map[string]map[string]any{}
	}
	if f.execution.batches[id] == nil {
		f.execution.batches[id] = newSubscriptionBatch(id, 1)
	}
	maps.Copy(f.execution.batches[id], fields)
}

func (f *fakeMoviePilot) subscriptionBatch(id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return maps.Clone(f.execution.batches[id])
}

func (f *fakeMoviePilot) servesSubscriptionExecution(w http.ResponseWriter, r *http.Request) bool {
	search := r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, subSearchPrefix)
	batch := strings.HasPrefix(r.URL.Path, subBatchPrefix)
	if !search && !batch {
		return false
	}
	response := f.takeSearchResponse(r.Method + " " + r.URL.Path)
	if writeSearchOverride(w, r, response) {
		return true
	}
	var result map[string]any
	if search {
		result = f.submitSubscriptionSearch(r)
	} else {
		result = f.readOrCancelSubscriptionBatch(r)
	}
	if !waitSearchResponse(r, response.hold) {
		return true
	}
	if response.lost {
		writeEnvelope(w, http.StatusOK, `{"success":`)
		return true
	}
	if response.body != "" {
		writeEnvelope(w, http.StatusOK, response.body)
		return true
	}
	body, _ := json.Marshal(result)
	writeEnvelope(w, http.StatusOK, string(body))
	return true
}

func waitSearchResponse(r *http.Request, held *heldCall) bool {
	if held == nil {
		return true
	}
	close(held.entered)
	defer close(held.exited)
	select {
	case <-held.release:
		return true
	case <-r.Context().Done():
		return false
	}
}

func (f *fakeMoviePilot) takeSearchResponse(key string) subscriptionSearchResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.execution.requests == nil {
		f.execution.requests = map[string]int{}
	}
	f.execution.requests[key]++
	response := f.execution.responses[key]
	delete(f.execution.responses, key)
	return response
}

func writeSearchOverride(w http.ResponseWriter, r *http.Request, response subscriptionSearchResponse) bool {
	if response.location != "" {
		http.Redirect(w, r, response.location, http.StatusTemporaryRedirect)
		return true
	}
	if response.status != 0 {
		writeEnvelope(w, response.status, response.body)
		return true
	}
	return false
}

func (f *fakeMoviePilot) submitSubscriptionSearch(r *http.Request) map[string]any {
	id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, subSearchPrefix))
	sub := f.editableSubscription(id)
	if sub == nil || sub["type"] != r.URL.Query().Get("type") {
		return searchEnvelope(false, "订阅不存在", nil)
	}
	body, _ := io.ReadAll(r.Body)
	if len(body) != 0 || len(r.URL.Query()) != 1 {
		f.t.Error("single-subscription search changed its public request contract")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for batchID, batch := range f.execution.batches {
		if batch["current_subscription_id"] == id && activeSearchBatch(batch) {
			return searchEnvelope(true, "已经在搜索中", searchSubmission(batchID, false))
		}
	}
	f.execution.created++
	batchID := fmt.Sprintf("search-batch-%d", f.execution.created)
	if f.execution.batches == nil {
		f.execution.batches = map[string]map[string]any{}
	}
	f.execution.batches[batchID] = newSubscriptionBatch(batchID, id)
	return searchEnvelope(true, "已安排搜索，很快开始", searchSubmission(batchID, true))
}

func activeSearchBatch(batch map[string]any) bool {
	return batch["state"] == "queued" || batch["state"] == "running" || batch["state"] == "cancelling"
}

func searchSubmission(id string, fresh bool) map[string]any {
	queued, ongoing := 0, 1
	if fresh {
		queued, ongoing = 1, 0
	}
	return map[string]any{
		"batch_id": id, "batch_ids": []string{id}, "single": true,
		"target_count": 1, "queued_count": queued, "ongoing_count": ongoing,
	}
}

func newSubscriptionBatch(id string, subID int) map[string]any {
	return map[string]any{
		"batch_id": id, "source": "manual", "state": "queued", "phase": "queued",
		"total_count": 1, "processed_count": 0, "finished_count": 0,
		"failed_count": 0, "cancelled_count": 0, "skipped_count": 0,
		"created_at": epoch.UTC().Format(time.RFC3339), "updated_at": epoch.UTC().Format(time.RFC3339),
		"current_subscription_id": subID, "current_site_id": nil, "error": nil, "can_cancel": true,
	}
}

func (f *fakeMoviePilot) readOrCancelSubscriptionBatch(r *http.Request) map[string]any {
	id := strings.TrimPrefix(r.URL.Path, subBatchPrefix)
	if r.Method == http.MethodPut && strings.HasSuffix(id, "/cancel") {
		return f.cancelSubscriptionSearch(strings.TrimSuffix(id, "/cancel"))
	}
	batch := f.subscriptionBatch(id)
	if r.Method != http.MethodGet || batch == nil {
		return searchEnvelope(false, "没有找到这次搜索", nil)
	}
	return searchEnvelope(true, "", batch)
}

func (f *fakeMoviePilot) cancelSubscriptionSearch(id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	batch := f.execution.batches[id]
	if batch == nil || (batch["state"] != "queued" && batch["state"] != "running" && batch["state"] != "cancelling") {
		return searchEnvelope(false, "这次搜索已经结束，暂时无法停止", nil)
	}
	batch["can_cancel"] = false
	if batch["state"] == "queued" {
		batch["state"], batch["phase"], batch["current_subscription_id"] = "cancelled", "cancelled", nil
		batch["processed_count"], batch["cancelled_count"] = 1, 1
	} else {
		batch["state"], batch["phase"] = "cancelling", "cancelling"
	}
	return searchEnvelope(true, "", nil)
}

func searchEnvelope(success bool, message string, data any) map[string]any {
	return map[string]any{"success": success, "message": message, "data": data}
}
