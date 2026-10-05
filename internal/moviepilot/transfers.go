package moviepilot

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"

	"github.com/SuperrNauto/Crychic/internal/notify"
)

const (
	transferPageSize = 50
	// maxTransferPages bounds one catch-up; older backlog is skipped.
	maxTransferPages = 20
	transferPath     = "/api/v1/history/transfer"
)

var (
	seasonPattern  = regexp.MustCompile(`^S(\d+)`)
	episodePattern = regexp.MustCompile(`^E(\d+)(?:-E(\d+))?`)
)

type transferRecord struct {
	ID          int    `json:"id"`
	MediaSource string `json:"media_source"`
	MediaID     string `json:"media_id"`
	Seasons     string `json:"seasons"`
	Episodes    string `json:"episodes"`
	Image       string `json:"image"`
}

// transferPage is one page of successful transfers, newest first.
func (c *Client) transferPage(ctx context.Context, page, size int) ([]transferRecord, error) {
	q := url.Values{"page": {strconv.Itoa(page)}, "count": {strconv.Itoa(size)}, "status": {"true"}}
	var data struct {
		List []transferRecord `json:"list"`
	}
	if err := c.do(ctx, call{method: http.MethodGet, path: transferPath, query: q}, &data); err != nil {
		return nil, err
	}
	return data.List, nil
}

func (c *Client) LatestTransfer(ctx context.Context) (int, error) {
	records, err := c.transferPage(ctx, 1, 1)
	if err != nil || len(records) == 0 {
		return 0, err
	}
	return records[0].ID, nil
}

// TransfersAfter pages back through history until it reaches id.
func (c *Client) TransfersAfter(ctx context.Context, id int) ([]notify.Transfer, error) {
	var out []notify.Transfer
	for page := 1; page <= maxTransferPages; page++ {
		records, err := c.transferPage(ctx, page, transferPageSize)
		if err != nil {
			return nil, err
		}
		for _, r := range records {
			if r.ID > id {
				out = append(out, r.toTransfer())
			}
		}
		if len(records) < transferPageSize || records[len(records)-1].ID <= id {
			break
		}
	}
	slices.SortFunc(out, func(a, b notify.Transfer) int { return a.ID - b.ID })
	return out, nil
}

// toTransfer parses MoviePilot's "S03" / "E01-E03" labels.
func (r transferRecord) toTransfer() notify.Transfer {
	t := notify.Transfer{ID: r.ID, Source: r.MediaSource, MediaID: r.MediaID, Image: r.Image}
	if m := seasonPattern.FindStringSubmatch(r.Seasons); m != nil {
		season, _ := strconv.Atoi(m[1])
		t.Season = &season
	}
	m := episodePattern.FindStringSubmatch(r.Episodes)
	if m == nil {
		return t
	}
	first, _ := strconv.Atoi(m[1])
	last := first
	if m[2] != "" {
		last, _ = strconv.Atoi(m[2])
	}
	for ep := first; ep <= last; ep++ {
		t.Episodes = append(t.Episodes, ep)
	}
	return t
}
