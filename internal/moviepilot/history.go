package moviepilot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

type historyRecord struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Year         string `json:"year"`
	Type         string `json:"type"`
	MediaSource  string `json:"media_source"`
	MediaID      string `json:"media_id"`
	Season       *int   `json:"season"`
	StartEpisode int    `json:"start_episode"`
	Poster       string `json:"poster"`
	Date         string `json:"date"`
}

// SubscriptionHistory reads MoviePilot's subscription history of one kind,
// newest first. Records without a media identity cannot be subscribed again
// and are left out.
func (c *Client) SubscriptionHistory(ctx context.Context, kind flow.Kind, count int) ([]flow.PastSubscription, error) {
	query := url.Values{"page": {"1"}, "count": {strconv.Itoa(count)}}
	var raw []json.RawMessage
	call := call{method: http.MethodGet, path: "/api/v1/subscribe/history/" + kindType(kind), query: query}
	if err := c.do(ctx, call, &raw); err != nil {
		return nil, err
	}
	out := make([]flow.PastSubscription, 0, min(len(raw), count))
	for _, record := range raw[:min(len(raw), count)] {
		var r historyRecord
		if err := json.Unmarshal(record, &r); err != nil {
			return nil, err
		}
		if r.MediaSource == "" || r.MediaID == "" {
			continue
		}
		out = append(out, flow.PastSubscription{
			ID: r.ID, Season: r.Season, StartEpisode: r.StartEpisode, Date: r.Date, Record: record,
			Media: flow.Media{
				Source: r.MediaSource, ID: r.MediaID, Title: r.Name, Year: r.Year,
				Kind: kind, PosterURL: posterURL(r.Poster),
			},
		})
	}
	return out, nil
}

// Resubscribe posts a history record back as a new subscription, the way
// MoviePilot's own WebUI does; MoviePilot drops the record's id and state
// fields and keeps its settings (sites, quality, start episode, ...).
func (c *Client) Resubscribe(ctx context.Context, past flow.PastSubscription) (int, error) {
	var created struct {
		ID *int `json:"id"`
	}
	body := json.RawMessage(past.Record)
	if err := c.do(ctx, call{method: http.MethodPost, path: "/api/v1/subscribe/", body: body}, &created); err != nil {
		return 0, err
	}
	return deref(created.ID), nil
}
