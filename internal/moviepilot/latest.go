package moviepilot

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

// latestCount is how many recent items each media server is asked for.
const latestCount = 20

type playItem struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"` // the year for movies and shows
	Type     string `json:"type"`
	Link     string `json:"link"` // the item's page in the server's web app
}

// Latest lists the newest items of every media server MoviePilot knows.
func (c *Client) Latest(ctx context.Context) ([]flow.LibraryItem, error) {
	var servers []struct {
		Name string `json:"name"`
	}
	if err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/mediaserver/clients"}, &servers); err != nil {
		return nil, err
	}
	var items []flow.LibraryItem
	for _, s := range servers {
		q := url.Values{"server": {s.Name}, "count": {strconv.Itoa(latestCount)}}
		var latest []playItem
		if err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/mediaserver/latest", query: q}, &latest); err != nil {
			return nil, err
		}
		for _, p := range latest {
			items = append(items, flow.LibraryItem{Title: p.Title, Year: p.Subtitle, Kind: p.Type, Server: s.Name, Link: p.Link})
		}
	}
	return items, nil
}
