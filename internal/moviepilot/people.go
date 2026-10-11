package moviepilot

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const (
	sourceBangumi = "bangumi"
	decimalBase   = 10
	float64Bits   = 64
	int64Bits     = 64
)

// discoverPaths are TMDB's condition searches by kind.
var discoverPaths = map[flow.Kind]string{
	flow.Movie: "/api/v1/discover/tmdb_movies",
	flow.TV:    "/api/v1/discover/tmdb_tvs",
}

// DiscoverBy searches TMDB by conditions; MoviePilot passes the rating,
// vote count and date on as lower bounds (vote_average.gte and so on).
func (c *Client) DiscoverBy(ctx context.Context, q flow.DiscoverQuery) ([]flow.Media, error) {
	path, ok := discoverPaths[q.Kind]
	if !ok {
		return nil, nil
	}
	query := url.Values{
		"sort_by": {q.Sort}, "with_genres": {q.Genre}, "with_original_language": {q.Language},
		"vote_average": {strconv.FormatFloat(q.MinRating, 'f', -1, float64Bits)}, "vote_count": {strconv.Itoa(q.MinVotes)},
		"release_date": {q.Since}, "page": {strconv.Itoa(max(q.Page, 1))},
	}
	return c.mediasWith(ctx, call{method: http.MethodGet, path: path, query: query})
}

// person is one entry of a cast list; Bangumi's is a voice actor, whose
// roles are the characters named in career.
type person struct {
	Source    string   `json:"source"`
	ID        any      `json:"id"`
	Name      string   `json:"name"`
	Character string   `json:"character"`
	Career    []string `json:"career"`
}

// Cast lists the people credited on TMDB or Bangumi media; other sources
// have none here.
func (c *Client) Cast(ctx context.Context, media flow.Media) ([]flow.Person, error) {
	var path string
	switch media.Source {
	case sourceTMDB:
		path = "/api/v1/tmdb/credits/" + url.PathEscape(media.ID) + "/" + url.PathEscape(kindType(media.Kind))
	case sourceBangumi:
		path = "/api/v1/bangumi/credits/" + url.PathEscape(media.ID)
	default:
		return nil, nil
	}
	var raw []person
	if err := c.do(ctx, call{method: http.MethodGet, path: path}, &raw); err != nil {
		return nil, err
	}
	out := make([]flow.Person, 0, len(raw))
	for _, p := range raw {
		id := personID(p.ID)
		if p.Source != media.Source || id == "" || p.Name == "" {
			continue
		}
		role := p.Character
		if role == "" {
			role = strings.Join(p.Career, " / ")
		}
		out = append(out, flow.Person{Source: p.Source, ID: id, Name: p.Name, Role: role})
	}
	return out, nil
}

// personID is a person's numeric id as text; anything else names nobody.
func personID(raw any) string {
	switch id := raw.(type) {
	case float64:
		if id > 0 && id == float64(int64(id)) {
			return strconv.FormatInt(int64(id), decimalBase)
		}
	case string:
		if n, err := strconv.ParseInt(id, decimalBase, int64Bits); err == nil && n > 0 {
			return id
		}
	}
	return ""
}

// PersonWorks lists the media a TMDB or Bangumi person is credited on, each
// once; Bangumi repeats a work for every role.
func (c *Client) PersonWorks(ctx context.Context, p flow.Person) ([]flow.Media, error) {
	if p.Source != sourceTMDB && p.Source != sourceBangumi {
		return nil, nil
	}
	path := "/api/v1/" + map[string]string{sourceTMDB: "tmdb", sourceBangumi: "bangumi"}[p.Source] +
		"/person/credits/" + url.PathEscape(p.ID)
	works, err := c.medias(ctx, path)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := works[:0]
	for _, m := range works {
		if key := m.Source + "/" + m.ID; !seen[key] {
			seen[key] = true
			out = append(out, m)
		}
	}
	return out, nil
}
