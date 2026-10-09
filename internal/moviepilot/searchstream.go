package moviepilot

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

var (
	errStreamAuth = &flow.UserError{Message: "资源 Cookie 已过期、被撤销或无权访问，请管理员安全更新配置后重启。普通 API Key 功能仍可使用；不会自动重新搜索。"}
	errStream     = &flow.UserError{Message: "资源搜索流已中断或返回了无法核验的数据，不会自动重新搜索。"}
)

func (c *Client) StreamsTorrents() bool { return c.cookie != "" }

// StreamTorrents owns one request and its full read budget. No raw response,
// frame, redirect URL or transport error can escape this credential boundary.
func (c *Client) StreamTorrents(ctx context.Context, search flow.TorrentSearch, publish func(flow.TorrentSnapshot)) error {
	ctx, cancel := context.WithTimeout(ctx, torrentSearchTimeout)
	defer cancel()
	resp, err := c.openSearchStream(ctx, search)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	state := streamResults{target: search.Target}
	frames := newSSEReader(resp.Body)
	for {
		raw, err := frames.next()
		if err != nil {
			return errStream
		}
		snapshot, changed, err := state.accept(raw)
		if err != nil {
			return errStream
		}
		if changed {
			publish(snapshot)
		}
		if snapshot.Final {
			return nil
		}
	}
}

func (c *Client) openSearchStream(ctx context.Context, search flow.TorrentSearch) (*http.Response, error) {
	if c.cookie == "" {
		return nil, errStreamAuth
	}
	path := "/api/v1/search/media/" + url.PathEscape(search.Target.Media.ID) + "/stream"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+torrentQuery(search).Encode(), nil)
	if err != nil {
		return nil, errStream
	}
	req.AddCookie(&http.Cookie{Name: "MoviePilot", Value: c.cookie})
	req.Header.Set("Accept", "text/event-stream")
	// Go retries bodyless GETs on stale pooled connections. This empty,
	// non-rewindable body marks a dispatched search as non-replayable, while
	// still sending an empty GET (the HTTP/1 transport probes it to EOF).
	req.Body = io.NopCloser(strings.NewReader(""))
	req.GetBody = nil
	resp, err := c.stream.Do(req)
	if err != nil {
		return nil, errStream
	}
	if err := searchStreamStatus(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}
	return resp, nil
}

func searchStreamStatus(resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errStreamAuth
	}
	typeName, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || typeName != "text/event-stream" || resp.StatusCode != http.StatusOK {
		return errStream
	}
	return nil
}

// A private sentinel avoids exposing decoder details from secret-bearing frames.
var errSSEProtocol = errors.New("invalid resource search stream")
