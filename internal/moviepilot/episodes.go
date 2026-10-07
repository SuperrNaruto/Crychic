package moviepilot

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

type episodeRecord struct {
	AirDate string `json:"air_date"`
	Number  int    `json:"episode_number"`
	Name    string `json:"name"`
}

// SeasonEpisodes lists a TMDB show's episodes of season with their air
// dates (GET /api/v1/tmdb/{tmdbid}/{season}).
func (c *Client) SeasonEpisodes(ctx context.Context, media flow.Media, season int) ([]flow.EpisodeAir, error) {
	if media.Source != sourceTMDB {
		return nil, errors.New("moviepilot: season episodes need a TMDB id")
	}
	path := "/api/v1/tmdb/" + url.PathEscape(media.ID) + "/" + strconv.Itoa(season)
	var records []episodeRecord
	if err := c.do(ctx, call{method: http.MethodGet, path: path}, &records); err != nil {
		return nil, err
	}
	episodes := make([]flow.EpisodeAir, 0, len(records))
	for _, r := range records {
		episodes = append(episodes, flow.EpisodeAir{Number: r.Number, Name: r.Name, Date: r.AirDate})
	}
	return episodes, nil
}
