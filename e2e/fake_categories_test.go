package e2e

import (
	"cmp"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	classificationPolicyPath = "GET /api/v1/media/classification/policy"
	policyFixture            = "classification_policy.json"
	categoryIDKey            = "media_category_id"
	categoryPathKey          = "media_category"

	// invalidCategoryMessage is MoviePilot v3.1.4's refusal, relayed with HTTP 200.
	invalidCategoryMessage = "订阅分类设置无效，请重新选择分类后重试"
)

type policyCategory struct {
	ID        string   `json:"id"`
	MediaType string   `json:"media_type"`
	Enabled   bool     `json:"enabled"`
	Path      []string `json:"path"`
}

// acceptedCategory is the resolver of v3.1.4: any enabled category of the
// subscription's media type, whatever its parent.
func (f *fakeMoviePilot) acceptedCategory(id, mediaType string) (policyCategory, bool) {
	f.mu.Lock()
	name := cmp.Or(f.routes[classificationPolicyPath].fixture, policyFixture)
	f.mu.Unlock()
	raw, err := os.ReadFile(filepath.Join("testdata", "moviepilot", name))
	if err != nil {
		f.t.Errorf("fixture: %v", err)
		return policyCategory{}, false
	}
	var policy struct {
		Data struct {
			Categories []policyCategory `json:"categories"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &policy) != nil {
		return policyCategory{}, false
	}
	for _, category := range policy.Data.Categories {
		if category.ID == id && category.Enabled && category.MediaType == mediaType {
			return category, true
		}
	}
	return policyCategory{}, false
}

func refuseCategory(w http.ResponseWriter) {
	writeEnvelope(w, http.StatusOK, `{"success":false,"message":"`+invalidCategoryMessage+`","data":null}`)
}

// checksCategory validates a created subscription's category like the
// service does; other creations continue to their route.
func (f *fakeMoviePilot) checksCategory(w http.ResponseWriter, r *http.Request) bool {
	if r.Method+" "+r.URL.Path != subscribePath {
		return false
	}
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	var sent struct {
		Type       string `json:"type"`
		CategoryID string `json:"media_category_id"`
	}
	if json.Unmarshal(body, &sent) != nil || sent.CategoryID == "" {
		return false
	}
	if _, valid := f.acceptedCategory(sent.CategoryID, sent.Type); valid {
		return false
	}
	refuseCategory(w)
	return true
}

// categoryRefused reports a partial update whose category is rejected; an
// update without the key is not validated, and "" returns to automatic.
func (f *fakeMoviePilot) categoryRefused(id int, patch map[string]any) bool {
	chosen, sent := patch[categoryIDKey].(string)
	sub := f.editableSubscription(id)
	if !sent || chosen == "" || sub == nil {
		return false
	}
	mediaType, _ := sub["type"].(string)
	_, valid := f.acceptedCategory(chosen, mediaType)
	return !valid
}

// storedCategory is the id and path snapshot a partial update leaves on the row.
func (f *fakeMoviePilot) storedCategory(id int, patch map[string]any) map[string]any {
	chosen, sent := patch[categoryIDKey].(string)
	if !sent {
		return nil
	}
	if chosen == "" {
		return map[string]any{categoryIDKey: nil, categoryPathKey: nil}
	}
	sub := f.editableSubscription(id)
	mediaType, _ := sub["type"].(string)
	category, _ := f.acceptedCategory(chosen, mediaType)
	return map[string]any{categoryIDKey: chosen, categoryPathKey: strings.Join(category.Path, "/")}
}
