package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// subscriptionStatePrefix is MoviePilot's PUT /api/v1/subscribe/status/{id}.
const subscriptionStatePrefix = "/api/v1/subscribe/status/"

// setsSubscriptionState answers a state change like MoviePilot: R, P or S
// for a subscription in the list, which then lists it in that state, and
// success false with 订阅不存在 for one that is not. A scenario route for
// the exact path overrides it.
func (f *fakeMoviePilot) setsSubscriptionState(w http.ResponseWriter, r *http.Request) bool {
	rest, found := strings.CutPrefix(r.URL.Path, subscriptionStatePrefix)
	f.mu.Lock()
	_, routed := f.routes[r.Method+" "+r.URL.Path]
	listed := f.routes[subsPath].fixture
	f.mu.Unlock()
	if r.Method != http.MethodPut || !found || routed {
		return false
	}
	id, _ := strconv.Atoi(rest)
	state := r.URL.Query().Get("state")
	if !slices.Contains([]string{"R", "P", "S"}, state) {
		writeEnvelope(w, http.StatusOK, `{"success":false,"message":"无效的订阅状态","data":null}`)
		return true
	}
	if !slices.Contains(subscriptionIDs(f, listed), id) {
		writeEnvelope(w, http.StatusOK, `{"success":false,"message":"订阅不存在","data":null}`)
		return true
	}
	f.mu.Lock()
	f.states[id] = state
	f.mu.Unlock()
	writeEnvelope(w, http.StatusOK, `{"success":true,"message":null,"data":null}`)
	return true
}

func subscriptionIDs(f *fakeMoviePilot, listed string) []int {
	var env struct {
		Data []struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	data, err := os.ReadFile(filepath.Join("testdata", "moviepilot", listed))
	if err != nil || json.Unmarshal(data, &env) != nil {
		f.t.Errorf("subscription list %q unreadable", listed)
	}
	var ids []int
	for _, s := range env.Data {
		ids = append(ids, s.ID)
	}
	return ids
}

// restated lists subscriptions in the states set since the scenario began.
func (f *fakeMoviePilot) restated(body string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var env map[string]any
	if len(f.states) == 0 || json.Unmarshal([]byte(body), &env) != nil {
		return body
	}
	subs, _ := env["data"].([]any)
	for _, s := range subs {
		sub, _ := s.(map[string]any)
		id, _ := sub["id"].(float64)
		if state, ok := f.states[int(id)]; ok {
			sub["state"] = state
		}
	}
	out, _ := json.Marshal(env)
	return string(out)
}
