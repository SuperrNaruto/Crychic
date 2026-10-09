package moviepilot

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

// BangumiSummary uses the API-key-protected MediaInfo projection. An empty
// overview is not a successful annotation and permits the public-API fallback.
func (c *Client) BangumiSummary(ctx context.Context, id string) (string, error) {
	number, err := strconv.Atoi(id)
	if err != nil || number <= 0 {
		return "", fmt.Errorf("moviepilot: invalid Bangumi subject id")
	}
	var info struct {
		Overview string `json:"overview"`
	}
	path := "/api/v1/bangumi/" + strconv.Itoa(number)
	if err := c.do(ctx, call{method: http.MethodGet, path: path}, &info); err != nil {
		return "", err
	}
	return unixLines(info.Overview), nil
}
