package telegram

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

const requestTimeout = 10 * time.Second

// Long polling uses the HTTP client's longer timeout. All other methods
// have a short deadline that includes reading the response body.
type deadlineClient struct{ http *http.Client }

func (c deadlineClient) Do(req *http.Request) (*http.Response, error) {
	if strings.HasSuffix(req.URL.Path, "/getUpdates") {
		return c.http.Do(req)
	}
	ctx, cancel := context.WithTimeout(req.Context(), requestTimeout)
	resp, err := c.http.Do(req.Clone(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error {
	defer b.cancel()
	return b.ReadCloser.Close()
}
