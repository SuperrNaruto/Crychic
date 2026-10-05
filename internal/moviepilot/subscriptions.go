package moviepilot

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

type subscriptionRecord struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Year        string `json:"year"`
	Type        string `json:"type"`
	MediaSource string `json:"media_source"`
	MediaID     string `json:"media_id"`
	Season      *int   `json:"season"`
	State       string `json:"state"`
	Lack        int    `json:"lack_episode"`
	Total       int    `json:"total_episode"`
	Poster      string `json:"poster"`
}

// Subscriptions lists every subscription; the API key is the superuser,
// who sees all of them.
func (c *Client) Subscriptions(ctx context.Context) ([]flow.Subscription, error) {
	var records []subscriptionRecord
	if err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/subscribe/"}, &records); err != nil {
		return nil, err
	}
	subs := make([]flow.Subscription, 0, len(records))
	for _, r := range records {
		kind := flow.Movie
		if r.Type == typeTV {
			kind = flow.TV
		}
		subs = append(subs, flow.Subscription{
			ID: r.ID, Title: r.Name, Year: r.Year, Kind: kind, Season: r.Season,
			State: r.State, Lack: r.Lack, Total: r.Total, Poster: r.Poster,
		})
	}
	return subs, nil
}

// Unsubscribe deletes a subscription; one already gone counts as deleted.
func (c *Client) Unsubscribe(ctx context.Context, id int) error {
	var result struct{}
	err := c.do(ctx, call{method: http.MethodDelete, path: "/api/v1/subscribe/" + strconv.Itoa(id)}, &result)
	var status *statusError
	if errors.As(err, &status) && status.code == http.StatusNotFound {
		return nil
	}
	return err
}
