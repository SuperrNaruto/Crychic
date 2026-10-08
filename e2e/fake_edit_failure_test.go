package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type editFailure struct {
	message int
	seen    chan struct{}
}

// failNextEdit gives one selected edit an authentic transient Bot API
// server-error response, without storing any part of the failed edit.
func (f *fakeTelegram) failNextEdit(message int) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failure = &editFailure{message: message, seen: make(chan struct{})}
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
	f.tr.add(fmt.Sprintf("<< editMessageText message=%d temporarily unavailable", id))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": false, "error_code": http.StatusServiceUnavailable, "description": "Service Unavailable",
	})
	close(failed.seen)
	return true
}
