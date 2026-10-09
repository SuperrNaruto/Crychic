package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type editFailure struct {
	message    int
	seen       chan struct{}
	retryAfter int
}

// failNextEdit gives one selected edit an authentic transient Bot API
// server-error response, without storing any part of the failed edit.
func (f *fakeTelegram) failNextEdit(message int) <-chan struct{} {
	return f.rejectNextEdit(message, 0)
}

func (f *fakeTelegram) rejectNextEdit(message, retryAfter int) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failure = &editFailure{message: message, seen: make(chan struct{}), retryAfter: retryAfter}
	return f.failure.seen
}

func (f *fakeTelegram) rejectEdit(w http.ResponseWriter, r *http.Request, method string) bool {
	if method != "editMessageText" {
		return false
	}
	id, _ := strconv.Atoi(r.FormValue("message_id"))
	f.mu.Lock()
	failed := f.failure
	if failed != nil && failed.message == id {
		f.failure = nil
	} else {
		failed = nil
	}
	f.mu.Unlock()
	if failed == nil {
		return false
	}
	status, description := http.StatusServiceUnavailable, "Service Unavailable"
	if failed.retryAfter > 0 {
		status, description = http.StatusTooManyRequests, "Too Many Requests"
		f.tr.add(fmt.Sprintf("<< editMessageText message=%d limited retry_after=%d", id, failed.retryAfter))
	} else {
		f.tr.add(fmt.Sprintf("<< editMessageText message=%d temporarily unavailable", id))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": false, "error_code": status, "description": description,
		"parameters": map[string]int{"retry_after": failed.retryAfter},
	})
	close(failed.seen)
	return true
}
