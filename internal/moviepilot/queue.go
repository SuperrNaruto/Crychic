package moviepilot

import (
	"context"
	"fmt"
	"net/http"

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
	Media *struct {
		MediaSource string `json:"media_source"`
		MediaID     string `json:"media_id"`
		Title       string `json:"title"`
		PosterPath  string `json:"poster_path"`
	} `json:"media"`
	Season *int `json:"season"`
	Tasks  []struct {
		State string `json:"state"`
		Meta  *struct {
			BeginEpisode *int `json:"begin_episode"`
		} `json:"meta"`
	} `json:"tasks"`
}

// Transfers lists the jobs in MoviePilot's transfer (整理) queue. Byte
// progress is only served to logged-in browsers, so files report state only.
func (c *Client) Transfers(ctx context.Context) ([]flow.TransferJob, error) {
	var jobs []queueJob
	if err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/transfer/queue"}, &jobs); err != nil {
		return nil, err
	}
	var out []flow.TransferJob
	for _, j := range jobs {
		if j.Media == nil || j.Media.MediaID == "" {
			continue
		}
		job := flow.TransferJob{
			ID:    fmt.Sprintf("%s:%s:%d", j.Media.MediaSource, j.Media.MediaID, deref(j.Season)),
			Title: j.Media.Title, Season: j.Season,
			Image: posterURL(j.Media.PosterPath),
		}
		for _, t := range j.Tasks {
			file := flow.TransferFile{State: fileStates[t.State]}
			if t.Meta != nil {
				file.Episode = deref(t.Meta.BeginEpisode)
			}
			job.Files = append(job.Files, file)
		}
		out = append(out, job)
	}
	return out, nil
}
