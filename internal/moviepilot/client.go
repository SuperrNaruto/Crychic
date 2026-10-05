// Package moviepilot is a minimal MoviePilot (v3 API) client implementing
// flow.Backend with the API key, which MoviePilot treats as its superuser.
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

	"github.com/SuperrNauto/Crychic/internal/flow"
)

const (
	searchCount  = 10
	maxErrorBody = 512

	typeMovie = "电影"
	typeTV    = "电视剧"
)

var errAuth = &flow.UserError{Message: "MoviePilot 拒绝了请求，请管理员检查 API Key。"}

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
	Type        string `json:"type"`
	Title       string `json:"title"`
	Year        string `json:"year"`
	MediaSource string `json:"media_source"`
	MediaID     string `json:"media_id"`
	Overview    string `json:"overview"`
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
		Source: i.MediaSource, ID: i.MediaID, Title: i.Title,
		Year: i.Year, Kind: kind, Overview: i.Overview,
	}, true
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

// IsSubscribed asks MoviePilot for an existing subscription; it answers with
// an empty subscription (null id) when there is none.
func (c *Client) IsSubscribed(ctx context.Context, t flow.Target) (bool, error) {
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
		return false, err
	}
	return sub.ID != nil, nil
}

type subscribeBody struct {
	Name        string `json:"name"`
	Year        string `json:"year,omitempty"`
	Type        string `json:"type"`
	MediaSource string `json:"media_source"`
	MediaID     string `json:"media_id"`
	Season      *int   `json:"season,omitempty"`
}

func (c *Client) Subscribe(ctx context.Context, t flow.Target) error {
	body := subscribeBody{
		Name: t.Media.Title, Year: t.Media.Year, Type: kindType(t.Media.Kind),
		MediaSource: t.Media.Source, MediaID: t.Media.ID, Season: t.Season,
	}
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := c.do(ctx, call{method: http.MethodPost, path: "/api/v1/subscribe/", body: body}, &resp); err != nil {
		return err
	}
	if resp.Success {
		return nil
	}
	if resp.Message == "" {
		resp.Message = "MoviePilot 未能创建订阅。"
	}
	return &flow.UserError{Message: resp.Message}
}

func kindType(k flow.Kind) string {
	if k == flow.TV {
		return typeTV
	}
	return typeMovie
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
	if resp.StatusCode/100 != 2 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return fmt.Errorf("moviepilot %s %s: status %d: %s", cl.method, cl.path, resp.StatusCode, snippet)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("moviepilot %s %s: decode: %w", cl.method, cl.path, err)
	}
	return nil
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
