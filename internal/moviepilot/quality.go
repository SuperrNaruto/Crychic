package moviepilot

import (
	"context"
	"net/http"
	"net/url"

	"github.com/SuperrNaruto/Crychic/internal/notify"
)

type metaInfo struct {
	Resolution string `json:"resource_pix"`
	Edition    string `json:"edition"`
	Type       string `json:"resource_type"`
	WebSource  string `json:"web_source"`
	Video      string `json:"video_encode"`
	Audio      string `json:"audio_encode"`
	Group      string `json:"resource_team"`
}

// Quality parses a release file name with MoviePilot's own recognizer, the
// one its WebUI shows search results with. It also looks the media up, and
// answers no meta_info when it cannot recognize the media.
func (c *Client) Quality(ctx context.Context, file string) (notify.Quality, error) {
	var data struct {
		Meta *metaInfo `json:"meta_info"`
	}
	q := url.Values{"title": {file}}
	if err := c.do(ctx, call{method: http.MethodGet, path: "/api/v1/media/recognize", query: q}, &data); err != nil {
		return notify.Quality{}, err
	}
	if data.Meta == nil {
		return notify.Quality{}, nil
	}
	m := data.Meta
	edition := m.Edition
	if edition == "" {
		edition = m.Type
	}
	return notify.Quality{
		Resolution: m.Resolution, Edition: edition, WebSource: m.WebSource,
		Video: m.Video, Audio: m.Audio, Group: m.Group,
	}, nil
}
