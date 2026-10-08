package moviepilot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
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
	Meta    *torrentMeta    `json:"meta_info"`
	Torrent json.RawMessage `json:"torrent_info"`
	Media   json.RawMessage `json:"media_info"`
}

type torrentMeta struct {
	metaInfo
	Season   *int  `json:"begin_season"`
	Episodes []int `json:"episode_list"`
}

// torrentFacts are the fields of torrent_info shown to pick a release.
type torrentFacts struct {
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
func (c *Client) SearchTorrents(ctx context.Context, t flow.Target) ([]flow.Torrent, error) {
	q := url.Values{"media_source": {t.Media.Source}, "mtype": {kindType(t.Media.Kind)}}
	if t.Season != nil {
		q.Set("season", strconv.Itoa(*t.Season))
	}
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

// toTorrent keeps results MoviePilot can download with media information.
func (rc torrentContext) toTorrent() (flow.Torrent, bool) {
	var facts torrentFacts
	var info mediaInfo
	if json.Unmarshal(rc.Torrent, &facts) != nil || json.Unmarshal(rc.Media, &info) != nil {
		return flow.Torrent{}, false
	}
	media, ok := info.toMedia()
	if !ok || facts.Title == "" {
		return flow.Torrent{}, false
	}
	record, err := json.Marshal(downloadBody{Media: rc.Media, Torrent: rc.Torrent})
	if err != nil {
		return flow.Torrent{}, false
	}
	found := flow.Torrent{
		Title: facts.Title, Description: facts.Description, Site: facts.Site, Size: facts.Size,
		Seeders: facts.Seeders, Promotion: facts.Promotion, HitAndRun: facts.HitAndRun,
		Published: facts.Published, Media: media, Record: record,
	}
	if m := rc.Meta; m != nil {
		found.Season, found.Episodes = m.Season, m.Episodes
		found.EpisodesUnsure = len(m.Episodes) > 0 && !namesEpisodes(facts.Title, m.Episodes)
		found.Resolution, found.Video, found.Group = m.Resolution, m.Video, m.Group
		found.Edition = m.Edition
		if found.Edition == "" {
			found.Edition = m.Type
		}
	}
	if media.Kind == flow.Movie {
		found.Season, found.Episodes, found.EpisodesUnsure = nil, nil, false
	}
	return found, true
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
