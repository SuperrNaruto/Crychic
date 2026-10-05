package moviepilot

import (
	"context"
	"fmt"
	"strings"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

// chartPaths are MoviePilot's recommendation lists behind each chart.
var chartPaths = map[flow.Chart]string{
	flow.Trending:   "/api/v1/recommend/tmdb_trending",
	flow.HotMovies:  "/api/v1/recommend/douban_movie_hot",
	flow.HotShows:   "/api/v1/recommend/douban_tv_hot",
	flow.InTheaters: "/api/v1/recommend/douban_showing",
}

// Discover reads a recommendation list; its entries are shaped like search
// results but may be Douban or Bangumi media.
func (c *Client) Discover(ctx context.Context, chart flow.Chart) ([]flow.Media, error) {
	path, ok := chartPaths[chart]
	if !ok {
		return nil, fmt.Errorf("moviepilot: unknown chart %d", chart)
	}
	medias, err := c.medias(ctx, path)
	if err != nil {
		return nil, err
	}
	for i, m := range medias {
		// Douban's overview is "2026 / 中国大陆 / 剧情 / director / cast";
		// the year is shown already.
		medias[i].Overview = strings.TrimPrefix(m.Overview, m.Year+" / ")
	}
	return medias, nil
}
