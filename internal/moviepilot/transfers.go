package moviepilot

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/SuperrNaruto/Crychic/internal/notify"
)

const (
	transferPageSize = 50
	// maxTransferPages bounds one catch-up; older backlog is skipped.
	maxTransferPages = 20
	transferPath     = "/api/v1/history/transfer"
)

var (
	seasonPattern  = regexp.MustCompile(`^S(\d+)`)
	episodePattern = regexp.MustCompile(`^E(\d+)(?:-E(\d+))?$`)
)

type transferRecord struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Year        string `json:"year"`
	MediaSource string `json:"media_source"`
	MediaID     string `json:"media_id"`
	Seasons     string `json:"seasons"`
	Episodes    string `json:"episodes"`
	Image       string `json:"image"`
	Src         string `json:"src"` // the downloaded file's server path
	Hash        string `json:"download_hash"`
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

// toTransfer parses MoviePilot's "S03" / "E01-E03" labels and keeps only
// the source file's name, never the server path.
func (r transferRecord) toTransfer() notify.Transfer {
	return notify.Transfer{
		ID: r.ID, Source: r.MediaSource, MediaID: r.MediaID, Title: r.Title, Year: r.Year, Image: r.Image,
		Season: parseSeason(r.Seasons), Episodes: parseEpisodes(r.Episodes),
		File: fileName(r.Src), Download: r.Hash,
	}
}

// fileName is the last element of a Linux or Windows path, "" for none.
func fileName(p string) string {
	name := path.Base(strings.ReplaceAll(p, `\`, "/"))
	if name == "." || name == "/" {
		return ""
	}
	return name
}

// parseSeason reads "S03" as 3, nil when there is no season.
func parseSeason(label string) *int {
	m := seasonPattern.FindStringSubmatch(label)
	if m == nil {
		return nil
	}
	season, _ := strconv.Atoi(m[1])
	return &season
}

// parseEpisodes reads MoviePilot's "E01-E03、E05" labels without filling gaps.
func parseEpisodes(label string) []int {
	var episodes []int
	for _, part := range strings.Split(label, "、") {
		m := episodePattern.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			continue
		}
		first, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		last := first
		if m[2] != "" {
			last, err = strconv.Atoi(m[2])
		}
		if err == nil {
			episodes = append(episodes, episodeRange(first, last)...)
		}
	}
	slices.Sort(episodes)
	return slices.Compact(episodes)
}

// episodeRange expands an inclusive range; invalid or unknown bounds stay empty.
func episodeRange(first, last int) []int {
	if first <= 0 || last < first {
		return nil
	}
	episodes := make([]int, last-first+1)
	for i := range episodes {
		episodes[i] = first + i
	}
	return episodes
}
