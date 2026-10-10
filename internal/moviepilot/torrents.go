package moviepilot

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// torrentSearchTimeout bounds a resource search: MoviePilot asks every
// indexer site while the request waits, which takes tens of seconds.
const torrentSearchTimeout = 2 * time.Minute

// torrentContext is one search result as MoviePilot serializes it
// (Context.to_dict). torrent_info carries the site's cookie and download
// link, so it is only ever kept raw and handed back unchanged.
type torrentContext struct {
	Meta    torrentMeta     `json:"meta_info"`
	Torrent json.RawMessage `json:"torrent_info"`
	Media   json.RawMessage `json:"media_info"`
	Matched *bool           `json:"media_info_is_target"`
	Match   string          `json:"match_status"`
}

type torrentMeta struct {
	metaInfo
	Season       *int   `json:"begin_season"`
	EndSeason    *int   `json:"end_season"`
	Seasons      []int  `json:"season_list"`
	Episodes     []int  `json:"episode_list"`
	EpisodeGroup string `json:"episode_group"`
}

// Season lists can default an unidentified TV release to S01 upstream;
// they never supply a missing begin_season or override contradictory facts.
func (m torrentMeta) rangeUnsure() bool {
	if m.EpisodeGroup != "" {
		return true
	}
	if m.EndSeason != nil && (m.Season == nil || *m.EndSeason != *m.Season) {
		return true
	}
	if len(m.Seasons) == 0 {
		return false
	}
	return m.Season == nil || len(m.Seasons) != 1 || m.Seasons[0] != *m.Season
}

// torrentFacts are the fields of torrent_info shown to pick a release.
type torrentFacts struct {
	SiteID      int     `json:"site"`
	PageURL     string  `json:"page_url"`
	Enclosure   string  `json:"enclosure"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Site        string  `json:"site_name"`
	Size        float64 `json:"size"`
	Seeders     int     `json:"seeders"`
	Promotion   string  `json:"volume_factor"`
	HitAndRun   bool    `json:"hit_and_run"`
	Published   string  `json:"pubdate"`
}

// downloadBody is what MoviePilot's WebUI posts to download a search result.
type downloadBody struct {
	Media   json.RawMessage `json:"media_in"`
	Torrent json.RawMessage `json:"torrent_in"`
}

// SearchTorrents searches the indexer sites for target by its identity, as
// the WebUI's 搜索资源 does. MoviePilot answers no result with success false
// and 未搜索到任何资源, which reaches the user verbatim.
func (c *Client) SearchTorrents(ctx context.Context, search flow.TorrentSearch) ([]flow.Torrent, error) {
	t := search.Target
	q := torrentQuery(search)
	var raw []torrentContext
	path := "/api/v1/search/media/" + url.PathEscape(t.Media.ID)
	if err := c.do(ctx, call{method: http.MethodGet, path: path, query: q, timeout: torrentSearchTimeout}, &raw); err != nil {
		return nil, err
	}
	out := make([]flow.Torrent, 0, len(raw))
	for _, rc := range raw {
		if found, ok := rc.toTorrent(); ok {
			out = append(out, found)
		}
	}
	return out, nil
}

func torrentQuery(search flow.TorrentSearch) url.Values {
	t := search.Target
	q := url.Values{"media_source": {t.Media.Source}, "mtype": {kindType(t.Media.Kind)}}
	if t.Season != nil {
		q.Set("season", strconv.Itoa(*t.Season))
	}
	var sites []string
	for _, id := range search.Sites {
		sites = append(sites, strconv.Itoa(id))
	}
	if len(sites) > 0 {
		q.Set("sites", strings.Join(sites, ","))
	}
	return q
}

// toTorrent keeps results MoviePilot can download with media information.
func (rc torrentContext) toTorrent() (flow.Torrent, bool) {
	if !rc.confirmedMedia() {
		return flow.Torrent{}, false
	}
	var facts torrentFacts
	var info struct {
		mediaInfo
		EpisodeGroup string `json:"episode_group"`
	}
	if json.Unmarshal(rc.Torrent, &facts) != nil || json.Unmarshal(rc.Media, &info) != nil {
		return flow.Torrent{}, false
	}
	media, ok := info.toMedia()
	if !ok || facts.Title == "" {
		return flow.Torrent{}, false
	}
	found, ok := rc.project(facts)
	found.Media = media
	found.RangeUnsure = found.RangeUnsure || info.EpisodeGroup != ""
	if media.Kind == flow.Movie {
		found.Season, found.Episodes, found.EpisodesUnsure = nil, nil, false
	}
	return found, ok
}

func (rc torrentContext) confirmedMedia() bool {
	return (rc.Matched == nil || *rc.Matched) && (rc.Match == "" || rc.Match == "exact")
}

func (rc torrentContext) previewTorrent() (flow.Torrent, bool) {
	var facts torrentFacts
	if json.Unmarshal(rc.Torrent, &facts) != nil || facts.Title == "" {
		return flow.Torrent{}, false
	}
	found, ok := rc.project(facts)
	found.Unverified = true
	return found, ok
}

func (rc torrentContext) project(facts torrentFacts) (flow.Torrent, bool) {
	record, err := json.Marshal(downloadBody{Media: rc.Media, Torrent: rc.Torrent})
	if err != nil {
		return flow.Torrent{}, false
	}
	m := rc.Meta
	found := flow.Torrent{
		Identity: torrentIdentity(facts, rc.Meta),
		Title:    facts.Title, Description: facts.Description, Site: facts.Site, Size: facts.Size,
		Seeders: facts.Seeders, Promotion: facts.Promotion, HitAndRun: facts.HitAndRun,
		Published: facts.Published, Record: record,
		Season: m.Season, Episodes: m.Episodes,
		EpisodesUnsure: len(m.Episodes) > 0 && !namesEpisodes(facts.Title, m.Episodes),
		RangeUnsure:    m.rangeUnsure(),
		Resolution:     m.Resolution, Video: m.Video, Group: m.Group, Edition: cmp.Or(m.Edition, m.Type),
	}
	return found, true
}

// Hash only a resource's identity/selection facts. Mutable seed counts and
// filtering priorities must not renumber it; a changed target gets a new ID.
func torrentIdentity(f torrentFacts, meta torrentMeta) string {
	raw, _ := json.Marshal(struct {
		SiteID               int
		Site, Page, Download string
		Title, Description   string
		Size                 float64
		Promotion            string
		HitAndRun            bool
		Meta                 torrentMeta
	}{f.SiteID, f.Site, f.PageURL, f.Enclosure, f.Title, f.Description, f.Size, f.Promotion, f.HitAndRun, meta})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

// Download posts a search result back the way the WebUI does; MoviePilot
// answers with the download's id (the torrent hash).
func (c *Client) Download(ctx context.Context, t flow.Torrent) (string, error) {
	var added struct {
		ID string `json:"download_id"`
	}
	body := json.RawMessage(t.Record)
	if err := c.do(ctx, call{method: http.MethodPost, path: "/api/v1/download/", body: body, secret: true}, &added); err != nil {
		return "", err
	}
	return added.ID, nil
}
