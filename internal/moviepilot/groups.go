package moviepilot

import (
	"context"
	"net/http"
	"net/url"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// episodeGroup is a TMDB episode group summary as MoviePilot lists it.
type episodeGroup struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	Seasons  int    `json:"group_count"`
	Episodes int    `json:"episode_count"`
}

// EpisodeGroups lists a TMDB show's episode groups; other media have none.
func (c *Client) EpisodeGroups(ctx context.Context, media flow.Media) ([]flow.EpisodeGroup, error) {
	if media.Source != sourceTMDB || media.Kind != flow.TV {
		return nil, nil
	}
	var raw []episodeGroup
	if err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/media/groups/" + url.PathEscape(media.ID)}, &raw); err != nil {
		return nil, err
	}
	out := make([]flow.EpisodeGroup, 0, len(raw))
	for _, g := range raw {
		if g.ID != "" {
			out = append(out, flow.EpisodeGroup{ID: g.ID, Name: g.Name, Type: g.Type, Seasons: g.Seasons, Episodes: g.Episodes})
		}
	}
	return out, nil
}

// GroupSeasons lists an episode group's seasons, numbered as the group
// numbers them.
func (c *Client) GroupSeasons(ctx context.Context, group string) ([]flow.Season, error) {
	var raw []struct {
		SeasonNumber *int   `json:"season_number"`
		Name         string `json:"name"`
		EpisodeCount int    `json:"episode_count"`
	}
	if err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/media/group/seasons/" + url.PathEscape(group)}, &raw); err != nil {
		return nil, err
	}
	seasons := make([]flow.Season, 0, len(raw))
	for _, s := range raw {
		if s.SeasonNumber != nil {
			seasons = append(seasons, flow.Season{Number: *s.SeasonNumber, Name: s.Name, EpisodeCount: s.EpisodeCount})
		}
	}
	return seasons, nil
}

// findGroupSubscription looks for a subscription to t's season in t's
// episode group; an empty group means standard seasons. MoviePilot's lookup
// by media filters by season number only, so a subscription in standard
// seasons or another group would pass for this one; the full list carries
// each subscription's group.
func (c *Client) findGroupSubscription(ctx context.Context, t flow.Target) (flow.Subscription, error) {
	subs, err := c.Subscriptions(ctx)
	if err != nil {
		return flow.Subscription{}, err
	}
	for _, s := range subs {
		if s.Source == t.Media.Source && s.MediaID == t.Media.ID && s.EpisodeGroup == t.EpisodeGroup &&
			s.Season != nil && t.Season != nil && *s.Season == *t.Season {
			return s, nil
		}
	}
	return flow.Subscription{}, nil
}
