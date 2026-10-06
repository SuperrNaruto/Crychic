// Package bangumi reads Bangumi's public API: the airing calendar and
// synopses. MoviePilot serves the calendar flattened, without the weekday
// each show airs on, and answers a failed Bangumi lookup with an empty
// result, so the bot asks Bangumi itself; everything else still goes
// through MoviePilot.
package bangumi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/SuperrNaruto/Crychic/internal/flow"
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

// errGone is a subject Bangumi does not have (any more).
var errGone = errors.New("bangumi: no such subject")

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
	var days []day
	if err := c.get(ctx, "/calendar", &days); err != nil {
		return nil, err
	}
	var picks []flow.Media
	for _, d := range days {
		for _, it := range d.Items {
			picks = append(picks, it.toMedia(d.Weekday.ID))
		}
	}
	return picks, nil
}

// Summary is the synopsis of subject id; "" when it has none or is gone.
func (c *Client) Summary(ctx context.Context, id string) (string, error) {
	var subject item
	err := c.get(ctx, "/v0/subjects/"+id, &subject)
	if errors.Is(err, errGone) {
		return "", nil
	}
	return synopsis(subject.Summary), err
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("bangumi GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errGone
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bangumi GET %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("bangumi GET %s: %w", path, err)
	}
	return nil
}

// toMedia names the show the way MoviePilot's Bangumi projection does
// (Chinese title, else the original), so search matches work alike.
func (it item) toMedia(weekday int) flow.Media {
	id := strconv.Itoa(it.ID)
	title := it.NameCN
	if title == "" {
		title = it.Name
	}
	year := ""
	if len(it.AirDate) >= yearDigits {
		year = it.AirDate[:yearDigits]
	}
	return flow.Media{
		Source: source, ID: id, Title: title, OriginalTitle: it.Name,
		Year: year, Kind: flow.TV, Rating: it.Rating.Score, PosterURL: it.Images.Large,
		Link: "https://bgm.tv/subject/" + id, Overview: synopsis(it.Summary),
		Released: it.AirDate, Weekday: weekday, CalendarID: id,
	}
}

// synopsis drops the carriage returns, indent and runs of blank lines
// Bangumi summaries carry.
func synopsis(s string) string {
	var lines []string
	blank := false
	for line := range strings.Lines(strings.ReplaceAll(s, "\r\n", "\n")) {
		line = strings.TrimRightFunc(line, unicode.IsSpace)
		if line == "" && blank {
			continue
		}
		blank = line == ""
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
