package moviepilot

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const failurePageSize = 50

var errFailureHistory = errors.New("moviepilot: failed transfer history is incomplete")
var exactFailureSeason = regexp.MustCompile(`^S\d+$`)

// recovery_action and errmsg are deliberately not decoded: they are free
// text, even though failure_stage and the retry flags are structured.
type failedTransferRecord struct {
	ID             int    `json:"id"`
	Source         string `json:"media_source"`
	MediaID        string `json:"media_id"`
	Type           string `json:"type"`
	Seasons        string `json:"seasons"`
	Episodes       string `json:"episodes"`
	Download       string `json:"download_hash"`
	EpisodeGroup   string `json:"episode_group"`
	Status         *bool  `json:"status"`
	Stage          string `json:"failure_stage"`
	RetryExhausted bool   `json:"retry_exhausted"`
	AutoPaused     bool   `json:"auto_paused"`
}

type failedTransferPage struct {
	List  []failedTransferRecord `json:"list"`
	Total *int                   `json:"total"`
}

// FailedTransfers reads a bounded diagnostic window. Earlier pages remain
// useful after a later failure, but cannot be called a complete history.
func (c *Client) FailedTransfers(ctx context.Context) (flow.FailedTransfers, error) {
	var out flow.FailedTransfers
	seen := map[int]bool{}
	read, total := 0, 0
	for page := 1; read < flow.FailedTransferLimit; page++ {
		data, err := c.failurePage(ctx, page)
		if err != nil {
			return out, err
		}
		if page > 1 && total != *data.Total {
			return out, errFailureHistory
		}
		total = *data.Total
		for _, raw := range data.List {
			item, valid := raw.failure()
			if valid && !seen[item.ID] {
				out.Items = append(out.Items, item)
				seen[item.ID] = true
			}
		}
		read += len(data.List)
		if read >= total {
			return out, nil
		}
		if len(data.List) < failurePageSize {
			return out, errFailureHistory
		}
	}
	out.Limited = true
	return out, nil
}

func (c *Client) failurePage(ctx context.Context, page int) (failedTransferPage, error) {
	q := url.Values{"status": {"false"}, "page": {strconv.Itoa(page)}, "count": {strconv.Itoa(failurePageSize)}}
	var data failedTransferPage
	err := c.do(ctx, call{method: http.MethodGet, path: transferPath, query: q, secret: true}, &data)
	if errors.Is(err, errAuth) {
		return data, errAuth
	}
	if err != nil || data.List == nil || data.Total == nil {
		return data, errFailureHistory
	}
	if *data.Total < len(data.List) || len(data.List) > failurePageSize {
		return data, errFailureHistory
	}
	return data, nil
}

func (r failedTransferRecord) failure() (flow.TransferFailure, bool) {
	kind, supported := mediaKinds[r.Type]
	if r.ID <= 0 || r.Status == nil || *r.Status || !supported || r.Source == "" || r.MediaID == "" {
		return flow.TransferFailure{}, false
	}
	season := parseSeason(r.Seasons)
	if !r.matchesKind(kind) {
		return flow.TransferFailure{}, false
	}
	return flow.TransferFailure{
		ID: r.ID, Source: r.Source, MediaID: r.MediaID, Kind: kind, Season: season,
		Episodes: parseEpisodes(r.Episodes), Download: r.Download, EpisodeGroup: r.EpisodeGroup,
		Stage: failureStages[r.Stage], RetryExhausted: r.RetryExhausted, AutoPaused: r.AutoPaused,
	}, true
}

func (r failedTransferRecord) matchesKind(kind flow.Kind) bool {
	if kind == flow.Movie {
		return r.Seasons == ""
	}
	return exactFailureSeason.MatchString(r.Seasons) && parseSeason(r.Seasons) != nil
}

var failureStages = map[string]flow.TransferFailureStage{
	"source_access": flow.FailureSource, "recognition": flow.FailureRecognition,
	"planning": flow.FailurePlanning, "destination_access": flow.FailureDestination,
	"transfer": flow.FailureTransfer, "overwrite": flow.FailureOverwrite,
	"downloader_cleanup": flow.FailureCleanup, "execution": flow.FailureExecution,
}
