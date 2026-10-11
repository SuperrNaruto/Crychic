package moviepilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// fileStates maps MoviePilot's transfer task states.
var fileStates = map[string]flow.FileState{
	"waiting":   flow.FileWaiting,
	"running":   flow.FileRunning,
	"completed": flow.FileDone,
	"failed":    flow.FileFailed,
}

type queueJob struct {
	Media struct {
		MediaSource string `json:"media_source"`
		MediaID     string `json:"media_id"`
		Type        string `json:"type"`
		Title       string `json:"title"`
		PosterPath  string `json:"poster_path"`
	} `json:"media"`
	Season *int `json:"season"`
	Tasks  []struct {
		State      string    `json:"state"`
		Meta       queueMeta `json:"meta"`
		Download   string    `json:"download_hash"`
		Downloader string    `json:"downloader"`
	} `json:"tasks"`
}

type queueMeta struct {
	BeginEpisode int  `json:"begin_episode"`
	EndEpisode   *int `json:"end_episode"`
}

func (m queueMeta) episodes() []int {
	first := m.BeginEpisode
	last := first
	if m.EndEpisode != nil {
		last = *m.EndEpisode
	}
	return episodeRange(first, last)
}

// Transfers lists the jobs in MoviePilot's transfer (整理) queue. Byte
// progress is only served to logged-in browsers, so files report state only.
func (c *Client) Transfers(ctx context.Context) ([]flow.TransferJob, error) {
	var jobs []queueJob
	if err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/transfer/queue"}, &jobs); err != nil {
		return nil, err
	}
	if jobs == nil {
		return nil, errors.New("moviepilot: missing transfer queue")
	}
	var out []flow.TransferJob
	for _, j := range jobs {
		if j.Media.MediaID == "" {
			continue
		}
		job := flow.TransferJob{
			ID:    fmt.Sprintf("%s:%s:%d", j.Media.MediaSource, j.Media.MediaID, deref(j.Season)),
			Title: j.Media.Title, Season: j.Season, Kind: mediaKinds[j.Media.Type],
			Image: posterURL(j.Media.PosterPath), Source: j.Media.MediaSource, MediaID: j.Media.MediaID,
		}
		for _, t := range j.Tasks {
			state, known := fileStates[t.State]
			if !known {
				state = flow.FileUnknown
			}
			file := flow.TransferFile{Episodes: t.Meta.episodes(), State: state, Download: t.Download, Downloader: t.Downloader}
			job.Files = append(job.Files, file)
		}
		out = append(out, job)
	}
	return out, nil
}

const (
	// progressTimeout keeps a task view from waiting on the optional plugin.
	progressTimeout = 5 * time.Second
	fullPercent     = 100
)

// fileProgress is one running file as the Crychic plugin projects it: the
// queue's identity and the host's own progress record, never a path.
type fileProgress struct {
	MediaSource string   `json:"media_source"`
	MediaID     string   `json:"media_id"`
	Season      *int     `json:"season"`
	Download    string   `json:"download_hash"`
	Downloader  string   `json:"downloader"`
	Active      *bool    `json:"active"`
	Percent     *float64 `json:"percent"`
	Begin       int      `json:"begin_episode"`
	End         *int     `json:"end_episode"`
}

// TransferProgress reads the running files' byte progress from the Crychic
// plugin; MoviePilot's own progress stream only accepts a browser cookie.
// Any failure, including a plugin without the route, is unknown progress.
func (c *Client) TransferProgress(ctx context.Context) ([]flow.FileProgress, error) {
	var out struct {
		Files []fileProgress `json:"files"`
	}
	cl := call{method: http.MethodGet, path: nativePrefix + "transfer-progress", timeout: progressTimeout}
	data, err := c.pluginCall(ctx, cl)
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(data, &out) != nil || out.Files == nil {
		return nil, errNativeRead
	}
	records := make([]flow.FileProgress, 0, len(out.Files))
	for _, f := range out.Files {
		if f.Active == nil || f.Percent == nil || *f.Percent < 0 || *f.Percent > fullPercent {
			continue
		}
		records = append(records, flow.FileProgress{
			Source: f.MediaSource, MediaID: f.MediaID, Season: f.Season,
			Episodes: queueMeta{BeginEpisode: f.Begin, EndEpisode: f.End}.episodes(),
			Download: f.Download, Downloader: f.Downloader, Active: *f.Active, Percent: *f.Percent,
		})
	}
	return records, nil
}
