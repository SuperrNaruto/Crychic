package moviepilot

import (
	"context"
	"net/http"
	"net/url"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

func (c *Client) SearchSites(ctx context.Context, kind flow.Kind) ([]flow.SearchSite, error) {
	var raw []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	path := "/api/v1/site/media/" + url.PathEscape(kindType(kind))
	if err := c.do(ctx, call{method: http.MethodGet, path: path, secret: true}, &raw); err != nil {
		return nil, &flow.UserError{Message: "暂时读不到可搜索站点，没有开始资源搜索哦～"}
	}
	var sites []flow.SearchSite
	seen := map[int]bool{}
	for _, site := range raw {
		if site.ID <= 0 || site.Name == "" || seen[site.ID] {
			continue
		}
		seen[site.ID] = true
		sites = append(sites, flow.SearchSite{ID: site.ID, Name: site.Name})
	}
	return sites, nil
}
