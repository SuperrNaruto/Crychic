package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// secondaryDownloader is a second qBittorrent instance next to the default.
const secondaryDownloader = "qBittorrent-secondary"

// fakeDownloaders are the instances the fake MoviePilot has configured.
var fakeDownloaders = map[string]bool{defaultDownloader: true, secondaryDownloader: true}

// torrentHash is v3.1.4's validate_torrent_hash: a BitTorrent v1 hash.
var torrentHash = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// controlsDownload answers PATCH /api/v1/download/{hash} like v3.1.4's
// update_task with action start/stop on qBittorrent: an unknown instance
// fails the operation, a hash the instance does not hold still succeeds,
// and success is the conjunction of the per-operation results. An exact
// scenario route overrides it.
func (f *fakeMoviePilot) controlsDownload(w http.ResponseWriter, r *http.Request) bool {
	hash, found := strings.CutPrefix(r.URL.Path, downloadTaskPrefix)
	f.mu.Lock()
	_, routed := f.routes[r.Method+" "+r.URL.Path]
	f.mu.Unlock()
	if r.Method != http.MethodPatch || !found || routed {
		return false
	}
	var sent struct {
		Action     *string `json:"action"`
		Downloader string  `json:"downloader"`
	}
	raw, _ := io.ReadAll(r.Body)
	if json.Unmarshal(raw, &sent) != nil || (sent.Action != nil && *sent.Action != "start" && *sent.Action != "stop") {
		writeEnvelope(w, http.StatusUnprocessableEntity, `{"detail":[{"type":"literal_error","loc":["body","action"]}]}`)
		return true
	}
	writeEnvelope(w, http.StatusOK, f.controlDownload(hash, sent.Action, sent.Downloader))
	return true
}

// controlDownload applies one validated request and returns its envelope.
func (f *fakeMoviePilot) controlDownload(hash string, action *string, downloader string) string {
	switch {
	case !torrentHash.MatchString(hash):
		return `{"success":false,"message":"hash 格式无效","data":null}`
	case action == nil:
		return `{"success":false,"message":"至少需要指定一个要更新的字段","data":null}`
	case downloader == "":
		// MoviePilot would pick the first instance holding the hash itself.
		f.t.Errorf("download control for %s names no downloader instance", hash)
		return `{"success":false,"message":"未找到下载任务或下载器不可用","data":null}`
	}
	stop := *action == "stop"
	verb := map[bool]string{true: "暂停", false: "开始"}[stop]
	applied := fakeDownloaders[downloader]
	message := "成功" + verb + "下载任务"
	if !applied {
		message = verb + "下载任务失败"
	} else {
		f.mu.Lock()
		f.paused[downloadKey{downloader: downloader, hash: hash}] = stop
		f.mu.Unlock()
	}
	return fmt.Sprintf(`{"success":%t,"message":null,"data":{"hash":%q,"downloader":%q,"results":[{"operation":%q,"success":%t,"message":%q}]}}`,
		applied, hash, downloader, *action, applied, message)
}
