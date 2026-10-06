// Package moviepilot is a minimal MoviePilot (v3 API) client implementing
// flow.Backend with the API key, which MoviePilot treats as its superuser.
// Every JSON endpoint answers with a {success, message, data} envelope.
package moviepilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const (
	searchCount  = 10
	maxErrorBody = 512

	typeMovie = "电影"
	typeTV    = "电视剧"

	// MoviePilot hands out full-size TMDB images (often several MB); chat
	// previews only need a poster-sized rendition.
	tmdbOriginalSize = "/t/p/original/"
	tmdbPosterSize   = "/t/p/w500/"
)

// invisible are marks Douban leaves around titles (e.g. U+200E).
const invisible = "\u200e\u200f "

var errAuth = &flow.UserError{Message: "MoviePilot 不让我进门…请管理员检查一下 API Key。"}

// Client talks to one MoviePilot instance.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, http: httpClient}
}

type mediaInfo struct {
	Type          string  `json:"type"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	Year          string  `json:"year"`
	MediaSource   string  `json:"media_source"`
	MediaID       string  `json:"media_id"`
	VoteAverage   float64 `json:"vote_average"`
	PosterPath    string  `json:"poster_path"`
	DetailLink    string  `json:"detail_link"`
	Overview      string  `json:"overview"`
	ReleaseDate   string  `json:"release_date"`
}

func (c *Client) Search(ctx context.Context, term string) ([]flow.Media, error) {
	q := url.Values{"title": {term}, "type": {"media"}, "count": {strconv.Itoa(searchCount)}}
	var infos []mediaInfo
	if err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/media/search", query: q}, &infos); err != nil {
		return nil, err
	}
	medias := make([]flow.Media, 0, len(infos))
	for _, info := range infos {
		if m, ok := info.toMedia(); ok {
			medias = append(medias, m)
		}
	}
	return medias, nil
}

// toMedia keeps only movies and shows that carry a subscribable identity.
func (i mediaInfo) toMedia() (flow.Media, bool) {
	kinds := map[string]flow.Kind{typeMovie: flow.Movie, typeTV: flow.TV}
	kind, ok := kinds[i.Type]
	if !ok || i.MediaSource == "" || i.MediaID == "" {
		return flow.Media{}, false
	}
	return flow.Media{
		Source: i.MediaSource, ID: i.MediaID, Title: strings.Trim(i.Title, invisible), OriginalTitle: i.OriginalTitle,
		Year: i.Year, Kind: kind, Rating: i.VoteAverage,
		PosterURL: posterURL(i.PosterPath),
		Link:      i.DetailLink, Overview: unixLines(i.Overview), Released: i.ReleaseDate,
	}, true
}

// posterURL is the poster rendition chat previews need.
func posterURL(path string) string {
	return strings.Replace(path, tmdbOriginalSize, tmdbPosterSize, 1)
}

// unixLines drops the carriage returns Bangumi synopses carry and the
// indent (often full-width spaces) some synopses start with.
func unixLines(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")) }

// named is a TMDB credit or genre; MoviePilot sends credits either as
// objects or as bare names.
type named struct{ Name string }

func (n *named) UnmarshalJSON(raw []byte) error {
	if len(raw) > 0 && raw[0] == '"' {
		return json.Unmarshal(raw, &n.Name)
	}
	var obj struct {
		Name string `json:"name"`
	}
	err := json.Unmarshal(raw, &obj)
	n.Name = obj.Name
	return err
}

func names(items []named) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it.Name != "" {
			out = append(out, it.Name)
		}
	}
	return out
}

func (c *Client) Details(ctx context.Context, media flow.Media) (flow.Details, error) {
	q := url.Values{"media_source": {media.Source}, "type_name": {kindType(media.Kind)}}
	var raw struct {
		Genres   []named `json:"genres"`
		Runtime  int     `json:"runtime"`
		Seasons  int     `json:"number_of_seasons"`
		Episodes int     `json:"number_of_episodes"`
		Actors   []named `json:"actors"`
		Overview string  `json:"overview"`
		Next     struct {
			Season  int `json:"season_number"`
			Episode int `json:"episode_number"`
		} `json:"next_episode_to_air"`
	}
	path := "/api/v1/media/" + url.PathEscape(media.ID)
	if err := c.do(ctx, call{method: http.MethodGet, path: path, query: q}, &raw); err != nil {
		return flow.Details{}, err
	}
	return flow.Details{
		Genres: names(raw.Genres), Runtime: raw.Runtime,
		Seasons: raw.Seasons, Episodes: raw.Episodes, Cast: names(raw.Actors),
		Next:     flow.Episode{Season: raw.Next.Season, Number: raw.Next.Episode},
		Overview: unixLines(raw.Overview),
	}, nil
}

func (c *Client) Seasons(ctx context.Context, media flow.Media) ([]flow.Season, error) {
	q := url.Values{"media_source": {media.Source}, "media_id": {media.ID}}
	var raw []struct {
		SeasonNumber *int   `json:"season_number"`
		Name         string `json:"name"`
		EpisodeCount int    `json:"episode_count"`
	}
	if err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/media/seasons", query: q}, &raw); err != nil {
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

// FindSubscription asks MoviePilot for an existing subscription; it answers
// with an empty subscription (null id) when there is none.
func (c *Client) FindSubscription(ctx context.Context, t flow.Target) (int, error) {
	q := url.Values{
		"media_source": {t.Media.Source},
		"title":        {t.Media.Title},
		"mtype":        {kindType(t.Media.Kind)},
	}
	if t.Media.Year != "" {
		q.Set("year", t.Media.Year)
	}
	if t.Season != nil {
		q.Set("season", strconv.Itoa(*t.Season))
	}
	var sub struct {
		ID *int `json:"id"`
	}
	path := "/api/v1/subscribe/media/" + url.PathEscape(t.Media.ID)
	if err := c.do(ctx, call{method: http.MethodGet, path: path, query: q}, &sub); err != nil {
		return 0, err
	}
	return deref(sub.ID), nil
}

// SubscriptionActive reports whether subscription id still exists; MoviePilot
// answers an unknown id with an empty subscription.
func (c *Client) SubscriptionActive(ctx context.Context, id int) (bool, error) {
	var sub struct {
		ID *int `json:"id"`
	}
	path := "/api/v1/subscribe/" + strconv.Itoa(id)
	if err := c.do(ctx, call{method: http.MethodGet, path: path}, &sub); err != nil {
		return false, err
	}
	return sub.ID != nil, nil
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

type subscribeBody struct {
	Name        string `json:"name"`
	Year        string `json:"year,omitempty"`
	Type        string `json:"type"`
	MediaSource string `json:"media_source"`
	MediaID     string `json:"media_id"`
	Season      *int   `json:"season,omitempty"`
	// StartEpisode makes MoviePilot skip the season's earlier episodes.
	StartEpisode int `json:"start_episode,omitempty"`
}

func (c *Client) Subscribe(ctx context.Context, t flow.Target) (int, error) {
	body := subscribeBody{
		Name: t.Media.Title, Year: t.Media.Year, Type: kindType(t.Media.Kind),
		MediaSource: t.Media.Source, MediaID: t.Media.ID, Season: t.Season,
		StartEpisode: t.StartEpisode,
	}
	var created struct {
		ID *int `json:"id"`
	}
	if err := c.do(ctx, call{method: http.MethodPost, path: "/api/v1/subscribe/", body: body}, &created); err != nil {
		return 0, err
	}
	return deref(created.ID), nil
}

func kindType(k flow.Kind) string {
	if k == flow.TV {
		return typeTV
	}
	return typeMovie
}

// statusError is a MoviePilot answer with a non-2xx status.
type statusError struct {
	call string
	code int
	body string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("moviepilot %s: status %d: %s", e.call, e.code, e.body)
}

// call is one MoviePilot API request.
type call struct {
	method string
	path   string
	query  url.Values
	body   any
}

func (c *Client) do(ctx context.Context, cl call, out any) error {
	req, err := c.newRequest(ctx, cl)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("moviepilot %s %s: %w", cl.method, cl.path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errAuth
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return &statusError{call: cl.method + " " + cl.path, code: resp.StatusCode, body: string(snippet)}
	}
	var env struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("moviepilot %s %s: decode: %w", cl.method, cl.path, err)
	}
	if !env.Success {
		return refusal(env.Message)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("moviepilot %s %s: decode data: %w", cl.method, cl.path, err)
	}
	return nil
}

// refusal turns an unsuccessful envelope (HTTP 200) into a user-facing error.
func refusal(message string) error {
	if message == "" {
		message = "MoviePilot 拒绝了这次操作…"
	}
	return &flow.UserError{Message: message}
}

func (c *Client) newRequest(ctx context.Context, cl call) (*http.Request, error) {
	target := c.baseURL + cl.path
	if len(cl.query) > 0 {
		target += "?" + cl.query.Encode()
	}
	var reader io.Reader
	if cl.body != nil {
		encoded, err := json.Marshal(cl.body)
		if err != nil {
			return nil, fmt.Errorf("moviepilot encode %s: %w", cl.path, err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, cl.method, target, reader)
	if err != nil {
		return nil, fmt.Errorf("moviepilot request %s: %w", cl.path, err)
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	if cl.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}
