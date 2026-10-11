package moviepilot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const statePaused = "paused"

// Numeric metadata IDs are scoped by media type, so movies and TV specials
// can share both a provider ID and season zero.
var mediaKinds = map[string]flow.Kind{typeMovie: flow.Movie, typeTV: flow.TV}

type downloadTask struct {
	Hash       string  `json:"hash"`
	Downloader string  `json:"downloader"`
	Size       float64 `json:"size"`     // bytes of the files being downloaded
	Progress   float64 `json:"progress"` // percent
	State      string  `json:"state"`    // downloading, paused
	Speed      string  `json:"dlspeed"`  // e.g. "3.1MB"
	LeftTime   string  `json:"left_time"`
	// Media is filled from MoviePilot's download history; tasks added to the
	// downloader by hand have none.
	Media struct {
		MediaSource string `json:"media_source"`
		MediaID     string `json:"media_id"`
		Type        string `json:"type"`
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
	if tasks == nil {
		return nil, errors.New("moviepilot: missing download list")
	}
	ambiguous := ambiguousDownloadInstances(tasks)
	var out []flow.Download
	for _, t := range tasks {
		if t.Media.MediaID == "" {
			continue
		}
		out = append(out, flow.Download{
			DownloadRef:       flow.DownloadRef{ID: t.Hash, Downloader: t.Downloader},
			InstanceAmbiguous: ambiguous[t.Hash],
			Source:            t.Media.MediaSource, MediaID: t.Media.MediaID, Kind: mediaKinds[t.Media.Type],
			Title: t.Media.Title, Image: t.Media.Image,
			Season: parseSeason(string(t.Media.Season)), Episodes: parseEpisodes(string(t.Media.Episode)),
			Size: t.Size, Progress: t.Progress, Paused: t.State == statePaused, Speed: t.Speed, Left: t.LeftTime,
		})
	}
	return out, nil
}

// Keep unknown-media records out of task lists, but do not let them hide
// an instance collision when another record has the requested media.
func ambiguousDownloadInstances(tasks []downloadTask) map[string]bool {
	instances := map[string]string{}
	ambiguous := map[string]bool{}
	for _, task := range tasks {
		previous, seen := instances[task.Hash]
		if task.Downloader == "" || (seen && previous != task.Downloader) {
			ambiguous[task.Hash] = true
		}
		instances[task.Hash] = task.Downloader
	}
	return ambiguous
}

// DeleteDownload removes a download from its own instance, with its files.
// Omitting name would address the default downloader, not the task's owner.
func (c *Client) DeleteDownload(ctx context.Context, download flow.DownloadRef) error {
	var none json.RawMessage
	q := url.Values{"name": {download.Downloader}}
	return c.do(ctx, call{method: http.MethodDelete, path: "/api/v1/download/" + url.PathEscape(download.ID), query: q}, &none)
}

const (
	controlStart = "start"
	controlStop  = "stop"
)

var errControlOutcome = errors.New("moviepilot: download control outcome unknown")

// downloadControl is v3.1.4's PATCH answer: one result per requested
// operation, with the instance MoviePilot actually used.
type downloadControl struct {
	Hash       string `json:"hash"`
	Downloader string `json:"downloader"`
	Results    []struct {
		Operation string `json:"operation"`
		Success   bool   `json:"success"`
	} `json:"results"`
}

// SetDownloadPaused stops or starts one download on its own instance. The
// downloader accepts an absent hash too, so callers read the list before and
// after; an empty instance would let MoviePilot pick one, so it is refused.
func (c *Client) SetDownloadPaused(ctx context.Context, download flow.DownloadRef, paused bool) error {
	action, verb := controlStart, "恢复"
	if paused {
		action, verb = controlStop, "暂停"
	}
	refused := &flow.UserError{Message: "MoviePilot 没能" + verb + "这个下载，请到 MoviePilot 核对任务哦～"}
	if download.ID == "" || download.Downloader == "" {
		return refused
	}
	client := *c
	client.http = c.stream // shared transport, no redirects or Cookie jar
	body := map[string]string{"action": action, "downloader": download.Downloader}
	cl := call{method: http.MethodPatch, path: "/api/v1/download/" + url.PathEscape(download.ID), body: body, secret: true}
	var result downloadControl
	err := client.do(ctx, cl, &result)
	switch {
	case err == nil && result.confirms(download, action):
		return nil
	case err == nil:
		return errControlOutcome
	case errors.Is(err, errAuth):
		return err
	case searchWriteRefused(err):
		return refused
	}
	return errControlOutcome
}

// confirms reports whether the answer is about this download on this
// instance and carries exactly the requested operation, successful; an
// empty result list would also count as success upstream.
func (d downloadControl) confirms(download flow.DownloadRef, action string) bool {
	return strings.EqualFold(d.Hash, download.ID) && d.Downloader == download.Downloader &&
		len(d.Results) == 1 && d.Results[0].Operation == action && d.Results[0].Success
}
