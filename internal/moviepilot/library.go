package moviepilot

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// libraryQuery identifies a title to MoviePilot's media server checks, which
// match by title and year as well as by id.
type libraryQuery struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Year        string `json:"year,omitempty"`
	MediaSource string `json:"media_source"`
	MediaID     string `json:"media_id"`
}

// Library asks the media server (Emby, Jellyfin, ...) behind MoviePilot what
// it already holds of media.
func (c *Client) Library(ctx context.Context, media flow.Media) (flow.Library, error) {
	q := libraryQuery{
		Type: kindType(media.Kind), Title: media.Title, Year: media.Year,
		MediaSource: media.Source, MediaID: media.ID,
	}
	if media.Kind == flow.TV {
		return c.heldEpisodes(ctx, q)
	}
	return c.heldMovie(ctx, q)
}

// heldEpisodes reads exists_remote, which maps season numbers to the episodes
// on the server and answers {} for a show it doesn't have.
func (c *Client) heldEpisodes(ctx context.Context, q libraryQuery) (flow.Library, error) {
	var raw map[string][]int
	if err := c.do(ctx, call{method: http.MethodPost, path: "/api/v1/mediaserver/exists_remote", body: q}, &raw); err != nil {
		return flow.Library{}, err
	}
	held := make(map[int][]int, len(raw))
	for season, episodes := range raw {
		n, err := strconv.Atoi(season)
		if err != nil {
			return flow.Library{}, fmt.Errorf("moviepilot: season %q in library answer", season)
		}
		held[n] = episodes
	}
	return flow.Library{Episodes: held}, nil
}

// heldMovie reads notexists: exists_remote answers {} for movies either way,
// while notexists lists nothing missing once the movie is on the server.
func (c *Client) heldMovie(ctx context.Context, q libraryQuery) (flow.Library, error) {
	var missing []struct{}
	if err := c.do(ctx, call{method: http.MethodPost, path: "/api/v1/mediaserver/notexists", body: q}, &missing); err != nil {
		return flow.Library{}, err
	}
	return flow.Library{Movie: len(missing) == 0}, nil
}
