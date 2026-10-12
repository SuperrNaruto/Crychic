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
	EpisodeGroup string `json:"episode_group"`
	Poster       string `json:"poster"`
	Date         string `json:"date"`
}

// SubscriptionHistory reads one page of MoviePilot's subscription history
// of a kind, newest first, with the whole history's size. Records without a
// media identity cannot be subscribed again and are left out of Past,
// counted by id in Unlisted.
func (c *Client) SubscriptionHistory(ctx context.Context, q flow.HistoryQuery) (flow.HistoryPage, error) {
	query := url.Values{"page": {strconv.Itoa(q.Page)}, "count": {strconv.Itoa(q.Count)}}
	var raw []json.RawMessage
	page := flow.HistoryPage{Total: -1}
	call := call{method: http.MethodGet, path: "/api/v1/subscribe/history/" + kindType(q.Kind), query: query, total: &page.Total}
	if err := c.do(ctx, call, &raw); err != nil {
		return flow.HistoryPage{}, err
	}
	raw = raw[:min(len(raw), q.Count)]
	page.Past = make([]flow.PastSubscription, 0, len(raw))
	for _, record := range raw {
		var r historyRecord
		if err := json.Unmarshal(record, &r); err != nil {
			return flow.HistoryPage{}, err
		}
		if r.MediaSource == "" || r.MediaID == "" {
			page.Unlisted = append(page.Unlisted, r.ID)
			continue
		}
		page.Past = append(page.Past, flow.PastSubscription{
			ID: r.ID, Season: r.Season, StartEpisode: r.StartEpisode, EpisodeGroup: r.EpisodeGroup,
			Date: r.Date, Record: record,
			Media: flow.Media{
				Source: r.MediaSource, ID: r.MediaID, Title: r.Name, Year: r.Year,
				Kind: q.Kind, PosterURL: posterURL(r.Poster),
			},
		})
	}
	return page, nil
}

// Resubscribe posts a history record back as a new subscription, the way
// MoviePilot's own WebUI does; MoviePilot drops the record's id and state
// fields and keeps its settings (sites, quality, start episode, ...).
func (c *Client) Resubscribe(ctx context.Context, past flow.PastSubscription) (int, error) {
	var created struct {
		ID int `json:"id"`
	}
	body := json.RawMessage(past.Record)
	if err := c.do(ctx, call{method: http.MethodPost, path: "/api/v1/subscribe/", body: body}, &created); err != nil {
		return 0, err
	}
	return created.ID, nil
}
