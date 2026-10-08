package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// rememberSubscription keeps successful creations and lookups available
// to GET /subscribe/{id}, with their real identity instead of just an id.
func (f *fakeMoviePilot) rememberSubscription(r *http.Request, response string) {
	var env struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if json.Unmarshal([]byte(response), &env) != nil || !env.Success {
		return
	}
	if r.Method == http.MethodDelete && isSubscriptionByID(r.URL.Path) {
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, subscriptionPath))
		f.mu.Lock()
		delete(f.subscribed, id)
		f.mu.Unlock()
		return
	}
	id, _ := env.Data["id"].(float64)
	if id <= 0 {
		return
	}
	record := env.Data
	key := r.Method + " " + r.URL.Path
	switch {
	case key == subscribePath:
		raw, _ := io.ReadAll(r.Body)
		if json.Unmarshal(raw, &record) != nil {
			f.t.Error("unreadable subscription request")
			return
		}
		record["id"] = id
	case strings.HasPrefix(key, "GET "+subscriptionPath+"media/"):
	default:
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.subscribed == nil {
		f.subscribed = map[int]map[string]any{}
	}
	f.subscribed[int(id)] = record
}
