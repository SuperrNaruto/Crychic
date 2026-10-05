package moviepilot

import (
	"cmp"
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

const (
	sourceTMDB     = "themoviedb"
	typeCollection = "系列"

	// seriesCandidates bounds how many collections found by one name are
	// opened to look for the movie.
	seriesCandidates = 3
)

// sequelMarks start a movie title's subtitle, e.g. 复仇者联盟4：终局之战 or
// Dune: Part Two; the series is named by what comes before.
const sequelMarks = "：:"

// sequelNumbers trail a numbered sequel's name, e.g. 沙丘2.
const sequelNumbers = "0123456789 "

// Related lists TMDB's recommendations for media; MoviePilot only knows them
// for TMDB media, so others have none.
func (c *Client) Related(ctx context.Context, media flow.Media) ([]flow.Media, error) {
	if media.Source != sourceTMDB {
		return nil, nil
	}
	path := "/api/v1/tmdb/recommend/" + url.PathEscape(media.ID) + "/" + url.PathEscape(kindType(media.Kind))
	return c.medias(ctx, path)
}

// Series lists the parts of the TMDB collection movie media belongs to, in
// release order. MoviePilot's details leave a movie's collection out, so
// collections are searched by the movie's names and the one listing the
// movie is taken; none found means none.
func (c *Client) Series(ctx context.Context, media flow.Media) ([]flow.Media, error) {
	if media.Source != sourceTMDB || media.Kind != flow.Movie {
		return nil, nil
	}
	tried := map[int]bool{}
	for _, name := range seriesNames(media) {
		ids, err := c.collections(ctx, name)
		if err != nil {
			return nil, err
		}
		parts, err := c.partsWith(ctx, media, unseen(ids, tried))
		if err != nil || len(parts) > 0 {
			return parts, err
		}
	}
	return nil, nil
}

// seriesNames are the names a movie's series may go by: its titles and
// their stems, without repeats.
func seriesNames(media flow.Media) []string {
	var names []string
	for _, title := range []string{media.Title, media.OriginalTitle} {
		for _, name := range []string{title, seriesStem(title)} {
			if name != "" && !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	return names
}

// seriesStem drops a sequel's subtitle and number: 沙丘2 → 沙丘.
func seriesStem(title string) string {
	if i := strings.IndexAny(title, sequelMarks); i > 0 {
		title = title[:i]
	}
	return strings.TrimRight(title, sequelNumbers)
}

// unseen keeps the ids not tried yet, marking them tried.
func unseen(ids []int, tried map[int]bool) []int {
	var out []int
	for _, id := range ids {
		if !tried[id] {
			tried[id] = true
			out = append(out, id)
		}
	}
	return out
}

// collections searches TMDB collections by name, best matches first.
func (c *Client) collections(ctx context.Context, name string) ([]int, error) {
	q := url.Values{"title": {name}, "type": {"collection"}, "count": {strconv.Itoa(searchCount)}}
	var raw []struct {
		Type         string `json:"type"`
		CollectionID int    `json:"collection_id"`
	}
	if err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/media/search", query: q}, &raw); err != nil {
		return nil, err
	}
	var ids []int
	for _, r := range raw {
		if r.Type == typeCollection && r.CollectionID != 0 && len(ids) < seriesCandidates {
			ids = append(ids, r.CollectionID)
		}
	}
	return ids, nil
}

// partsWith returns the parts of the first collection listing media.
func (c *Client) partsWith(ctx context.Context, media flow.Media, ids []int) ([]flow.Media, error) {
	for _, id := range ids {
		parts, err := c.medias(ctx, "/api/v1/tmdb/collection/"+strconv.Itoa(id))
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(parts, func(m flow.Media) bool { return m.Source == media.Source && m.ID == media.ID }) {
			slices.SortStableFunc(parts, func(a, b flow.Media) int { return cmp.Compare(releaseKey(a), releaseKey(b)) })
			return parts, nil
		}
	}
	return nil, nil
}

// releaseKey orders by release date, unknown dates last.
func releaseKey(m flow.Media) string {
	if m.Released == "" {
		return "~"
	}
	return m.Released
}

// medias reads a search-shaped list, keeping the subscribable entries.
func (c *Client) medias(ctx context.Context, path string) ([]flow.Media, error) {
	var infos []mediaInfo
	if err := c.do(ctx, call{method: http.MethodGet, path: path}, &infos); err != nil {
		return nil, err
	}
	out := make([]flow.Media, 0, len(infos))
	for _, info := range infos {
		if m, ok := info.toMedia(); ok {
			out = append(out, m)
		}
	}
	return out, nil
}
