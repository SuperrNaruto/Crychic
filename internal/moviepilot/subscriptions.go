package moviepilot

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

type subscriptionRecord struct {
	ID           int      `json:"id"`
	Date         string   `json:"date"`
	Name         string   `json:"name"`
	Year         string   `json:"year"`
	Type         string   `json:"type"`
	MediaSource  string   `json:"media_source"`
	MediaID      string   `json:"media_id"`
	Season       *int     `json:"season"`
	State        string   `json:"state"`
	Lack         int      `json:"lack_episode"`
	Total        int      `json:"total_episode"`
	StartEpisode int      `json:"start_episode"`
	Poster       string   `json:"poster"`
	Quality      string   `json:"quality"`
	Resolution   string   `json:"resolution"`
	Effect       string   `json:"effect"`
	FilterGroups []string `json:"filter_groups"`
	BestVersion  int      `json:"best_version"`
	LastSearch   string   `json:"last_search"`
	EpisodeGroup string   `json:"episode_group"`
	Execution    *struct {
		State   string `json:"state"`
		NextRun string `json:"next_run_at"`
		Error   string `json:"error"`
	} `json:"execution_status"`
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
		subs = append(subs, r.subscription())
	}
	return subs, nil
}

func (r subscriptionRecord) subscription() flow.Subscription {
	kind := flow.Movie
	if r.Type == typeTV {
		kind = flow.TV
	}
	s := flow.Subscription{
		ID: r.ID, Created: r.Date, Source: r.MediaSource, MediaID: r.MediaID,
		Title: r.Name, Year: r.Year, Kind: kind, Season: r.Season,
		State: r.State, Lack: r.Lack, Total: r.Total, StartEpisode: r.StartEpisode, Poster: posterURL(r.Poster),
		Quality: r.Quality, Resolution: r.Resolution, Effect: r.Effect,
		FilterGroups: r.FilterGroups, BestVersion: r.BestVersion != 0, LastSearch: subscriptionTime(r.LastSearch),
		EpisodeGroup: r.EpisodeGroup,
	}
	if r.Execution != nil {
		s.Execution = &flow.SubscriptionExecution{
			State: r.Execution.State, NextRun: subscriptionTime(r.Execution.NextRun),
			HasError: r.Execution.Error != "",
		}
	}
	return s
}

// Search timestamps are UTC, with or without an explicit RFC3339 offset.
// Missing or unrecognized optional timestamps do not break the subscription list.
func subscriptionTime(raw string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.DateTime, "2006-01-02T15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

// SetSubscriptionPaused sets a subscription's state to S (paused) or R
// (subscribed); MoviePilot answers success false for one that is gone.
func (c *Client) SetSubscriptionPaused(ctx context.Context, id int, paused bool) error {
	state := "R"
	if paused {
		state = "S"
	}
	q := url.Values{"state": {state}}
	var result struct{}
	return c.do(ctx, call{method: http.MethodPut, path: "/api/v1/subscribe/status/" + strconv.Itoa(id), query: q}, &result)
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
