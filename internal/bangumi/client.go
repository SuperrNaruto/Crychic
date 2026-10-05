// Package bangumi reads Bangumi's airing calendar. MoviePilot serves the
// same calendar flattened, without the weekday each show airs on, so the
// bot asks Bangumi itself; everything else still goes through MoviePilot.
package bangumi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

const (
	// userAgent names the bot, as Bangumi's API asks of every client.
	userAgent = "SuperrNaruto/Crychic (https://github.com/SuperrNaruto/Crychic)"
	// source is how MoviePilot names Bangumi media, so picks look up
	// details through it like any other Bangumi media.
	source = "bangumi"
	// yearDigits is the year prefix of an air date such as 2026-10-05.
	yearDigits = 4
)

// Client reads Bangumi's public API; it needs no key.
type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

type day struct {
	Weekday struct {
		ID int `json:"id"` // 1 Monday … 7 Sunday
	} `json:"weekday"`
	Items []item `json:"items"`
}

type item struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	NameCN  string `json:"name_cn"`
	Summary string `json:"summary"`
	AirDate string `json:"air_date"`
	Rating  struct {
		Score float64 `json:"score"`
	} `json:"rating"`
	Images struct {
		Large string `json:"large"`
	} `json:"images"`
}

// Calendar lists this season's airing shows, Monday's first, each with the
// weekday it airs on.
func (c *Client) Calendar(ctx context.Context) ([]flow.Media, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/calendar", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bangumi calendar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bangumi calendar: status %d", resp.StatusCode)
	}
	var days []day
	if err := json.NewDecoder(resp.Body).Decode(&days); err != nil {
		return nil, fmt.Errorf("bangumi calendar: %w", err)
	}
	var picks []flow.Media
	for _, d := range days {
		for _, it := range d.Items {
			picks = append(picks, it.toMedia(d.Weekday.ID))
		}
	}
	return picks, nil
}

// toMedia names the show the way MoviePilot's Bangumi projection does
// (Chinese title, else the original), so search matches work alike.
func (it item) toMedia(weekday int) flow.Media {
	title := it.NameCN
	if title == "" {
		title = it.Name
	}
	year := ""
	if len(it.AirDate) >= yearDigits {
		year = it.AirDate[:yearDigits]
	}
	return flow.Media{
		Source: source, ID: strconv.Itoa(it.ID), Title: title, OriginalTitle: it.Name,
		Year: year, Kind: flow.TV, Rating: it.Rating.Score, PosterURL: it.Images.Large,
		Link:     "https://bgm.tv/subject/" + strconv.Itoa(it.ID),
		Overview: strings.TrimSpace(strings.ReplaceAll(it.Summary, "\r\n", "\n")),
		Released: it.AirDate, Weekday: weekday,
	}
}
