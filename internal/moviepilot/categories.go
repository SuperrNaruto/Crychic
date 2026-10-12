package moviepilot

import (
	"context"
	"net/http"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const classificationPolicyPath = "/api/v1/media/classification/policy"

// SubscriptionCategories lists the enabled categories of a media kind from
// the active classification policy, in policy order. The ID is what a
// subscription write accepts; the name is display text.
func (c *Client) SubscriptionCategories(ctx context.Context, kind flow.Kind) ([]flow.SubscriptionCategory, error) {
	var policy struct {
		Categories []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			MediaType string `json:"media_type"`
			Enabled   bool   `json:"enabled"`
		} `json:"categories"`
	}
	if err := c.do(ctx, call{method: http.MethodGet, path: classificationPolicyPath}, &policy); err != nil {
		return nil, &flow.UserError{Message: "MoviePilot 的分类暂时读不到，自动分类照常使用哦～"}
	}
	var out []flow.SubscriptionCategory
	for _, category := range policy.Categories {
		if !category.Enabled || category.ID == "" || category.Name == "" || category.MediaType != kindType(kind) {
			continue
		}
		out = append(out, flow.SubscriptionCategory{ID: category.ID, Name: category.Name})
	}
	return out, nil
}
