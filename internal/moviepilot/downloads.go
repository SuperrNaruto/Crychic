package moviepilot

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const statePaused = "paused"

type downloadTask struct {
	Hash     string  `json:"hash"`
	Progress float64 `json:"progress"` // percent
	State    string  `json:"state"`    // downloading, paused
	Speed    string  `json:"dlspeed"`  // e.g. "3.1MB"
	LeftTime string  `json:"left_time"`
	// Media is filled from MoviePilot's download history; tasks added to the
	// downloader by hand have none.
	Media *struct {
		MediaSource string `json:"media_source"`
		MediaID     string `json:"media_id"`
		Title       string `json:"title"`
		Season      label  `json:"season"`
		Episode     label  `json:"episode"`
		Image       string `json:"image"`
	} `json:"media"`
}

// label is a history label such as "S01" or "E10-E12". The schema also
// allows numbers and lists; those carry nothing to show, so they read as "".
type label string

func (l *label) UnmarshalJSON(raw []byte) error {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		*l = label(s)
	}
	return nil
}

// Downloads lists the downloader's unfinished tasks that MoviePilot knows
// the media of; torrents added by hand are left out.
func (c *Client) Downloads(ctx context.Context) ([]flow.Download, error) {
	var tasks []downloadTask
	if err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/download/"}, &tasks); err != nil {
		return nil, err
	}
	var out []flow.Download
	for _, t := range tasks {
		if t.Media == nil || t.Media.MediaID == "" {
			continue
		}
		out = append(out, flow.Download{
			ID: t.Hash, Source: t.Media.MediaSource, MediaID: t.Media.MediaID,
			Title: t.Media.Title, Image: t.Media.Image,
			Season: parseSeason(string(t.Media.Season)), Episodes: parseEpisodes(string(t.Media.Episode)),
			Progress: t.Progress, Paused: t.State == statePaused, Speed: t.Speed, Left: t.LeftTime,
		})
	}
	return out, nil
}
