package e2e

import (
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"slices"
)

const (
	editSubscriptionPath = "PUT /api/v1/subscribe/"
	editGroupsPath       = "GET /api/v1/rule/groups"
	editBuiltinPath      = "GET /api/v1/rule/builtin"
	editCustomPath       = "GET /api/v1/rule/custom"
)

// The update endpoint persists only public, explicitly supplied fields.
// A lost response may follow a committed update; it is not a refusal.
type subscriptionEditResponse struct {
	status    int
	body      string
	location  string
	lost      bool
	readFails bool
}

func (f *fakeMoviePilot) editResponse(response subscriptionEditResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.editReply = response
}

func (f *fakeMoviePilot) editsSubscription(w http.ResponseWriter, r *http.Request) bool {
	if r.Method+" "+r.URL.Path != editSubscriptionPath {
		return false
	}
	f.mu.Lock()
	response := f.editReply
	f.editReply = subscriptionEditResponse{}
	f.mu.Unlock()
	if response.location != "" {
		http.Redirect(w, r, response.location, http.StatusTemporaryRedirect)
		return true
	}
	if response.status != 0 {
		writeEnvelope(w, response.status, response.body)
		return true
	}
	var patch map[string]any
	if json.NewDecoder(r.Body).Decode(&patch) != nil {
		writeEnvelope(w, http.StatusUnprocessableEntity, `{"success":false,"message":"invalid body","data":null}`)
		return true
	}
	id, _ := patch["id"].(float64)
	if !f.applySubscriptionEdit(int(id), patch) {
		writeEnvelope(w, http.StatusOK, `{"success":false,"message":"订阅不存在","data":null}`)
		return true
	}
	if response.readFails {
		f.setRoute(subsPath, route{status: http.StatusServiceUnavailable, fixture: "server_error.json"})
	}
	if response.lost {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":`)
		return true
	}
	writeEnvelope(w, http.StatusOK, `{"success":true,"message":null,"data":null}`)
	return true
}

func (f *fakeMoviePilot) applySubscriptionEdit(id int, patch map[string]any) bool {
	sub := f.editableSubscription(id)
	if sub == nil {
		return false
	}
	fields := maps.Clone(patch)
	for _, key := range []string{"id", "state", "note", "lack_episode", "current_priority", "episode_priority", "date", "username"} {
		delete(fields, key)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.edited == nil {
		f.edited = map[int]map[string]any{}
	}
	if f.edited[id] == nil {
		f.edited[id] = map[string]any{}
	}
	maps.Copy(f.edited[id], fields)
	f.editPatches = append(f.editPatches, maps.Clone(patch))
	return true
}

func (f *fakeMoviePilot) editableSubscription(id int) map[string]any {
	f.mu.Lock()
	listed := f.routes[subsPath].fixture
	f.mu.Unlock()
	var envelope struct {
		Data []map[string]any `json:"data"`
	}
	if listed == "" || json.Unmarshal([]byte(f.restated(string(fixture(f, listed)))), &envelope) != nil {
		return nil
	}
	for _, sub := range envelope.Data {
		if number, _ := sub["id"].(float64); int(number) == id {
			return sub
		}
	}
	return nil
}

func (f *fakeMoviePilot) subscriptionPatches() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.editPatches)
}
