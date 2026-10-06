package moviepilot

import (
	"context"
	"maps"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

type resourceFile struct {
	Title      string `json:"torrent_title"`
	Site       string `json:"site_name"`
	Downloader string `json:"downloader"`
	Hash       string `json:"hash"`
	Path       string `json:"file_path"`
}

type resourceEpisode struct {
	Downloads []resourceFile `json:"download"`
}

// Resources reads MoviePilot's per-subscription file records. The upstream
// joins by media identity and season, including earlier downloads of it.
func (c *Client) Resources(ctx context.Context, id int) ([]flow.Resource, error) {
	var data struct {
		Subscribe *struct{ ID int }       `json:"subscribe"`
		Episodes  map[int]resourceEpisode `json:"episodes"`
	}
	call := call{method: http.MethodGet, path: "/api/v1/subscribe/files/" + strconv.Itoa(id)}
	if err := c.do(ctx, call, &data); err != nil {
		return nil, err
	}
	if data.Subscribe == nil || data.Subscribe.ID != id {
		return nil, &flow.UserError{Message: "暂时查不到这条订阅的资源，可能已经完成或被取消啦～"}
	}
	return collectResources(data.Episodes), nil
}

// A pack shared by several episodes is one resource. Different downloaders
// remain separate even when their torrent hashes are identical.
func collectResources(episodes map[int]resourceEpisode) []flow.Resource {
	type key struct{ hash, title, site, downloader string }
	indices := map[key]int{}
	var out []flow.Resource
	for _, ep := range slices.Sorted(maps.Keys(episodes)) {
		for _, file := range episodes[ep].Downloads {
			k := key{file.Hash, file.Title, file.Site, file.Downloader}
			i, found := indices[k]
			if !found {
				i = len(out)
				indices[k] = i
				out = append(out, flow.Resource{
					Name: file.Title, Site: file.Site, Downloader: file.Downloader, DownloadID: file.Hash,
				})
			}
			addResourceFile(&out[i], file, ep)
		}
	}
	return out
}

func addResourceFile(resource *flow.Resource, file resourceFile, episode int) {
	if episode > 0 && !slices.Contains(resource.Episodes, episode) {
		resource.Episodes = append(resource.Episodes, episode)
	}
	name := path.Base(strings.ReplaceAll(file.Path, `\`, "/"))
	if name != "." && name != "/" && !slices.Contains(resource.Files, name) {
		resource.Files = append(resource.Files, name)
	}
}
