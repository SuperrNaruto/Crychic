package e2e

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	mpAPIKey = "mp-test-key"

	transferPath     = "/api/v1/history/transfer"
	subscriptionPath = "/api/v1/subscribe/"

	libraryShowPath  = "POST /api/v1/mediaserver/exists_remote"
	libraryMoviePath = "POST /api/v1/mediaserver/notexists"
	downloadsPath    = "GET /api/v1/download/"
	queuePath        = "GET /api/v1/transfer/queue"
	clientsPath      = "GET /api/v1/mediaserver/clients"
	latestPath       = "GET /api/v1/mediaserver/latest"
	recognizePath    = "GET /api/v1/media/recognize"
	ruleGroupsPath   = "GET /api/v1/system/setting/SubscribeFilterRuleGroups"
	schedulePath     = "GET /api/v1/dashboard/schedule"

	downloadPath       = "POST /api/v1/download/"
	downloadTaskPrefix = "/api/v1/download/"

	// siteCookie is the indexer site cookie in the torrent search fixtures;
	// it must come back in a download request and never reach a chat.
	siteCookie = "uid=10001; pass=fake-cookie-never-shown"

	createdFixture = "subscribe_created.json"
	createdID      = `"id": 1`
)

// idleServer answers the downloader checks the way MoviePilot does with
// nothing downloading, names its one media server, and keeps subscriptions
// going as the owner's instance does (global rule groups, subscription
// refresh on, scheduled search off); scenarios override them. The library itself is served from the transfers (fake_library_test).
var idleServer = map[string]route{
	downloadsPath:  ok("downloads_none.json"),
	queuePath:      ok("queue_none.json"),
	clientsPath:    ok("mediaserver_clients.json"),
	ruleGroupsPath: ok("subscribe_filter_groups.json"),
	schedulePath:   ok("schedule.json"),
}

// polledPaths are read over and over, by live task views and by the
// notifier checking the media server; like the arrival poll they stay out
// of the transcript, which records what users see.
var polledPaths = map[string]bool{
	downloadsPath: true, queuePath: true, clientsPath: true,
	libraryShowPath: true, libraryMoviePath: true, latestPath: true,
}

// route is a canned MoviePilot answer. Fixtures are trimmed recordings from a
// live v3.1.0 instance, except subscribe_rejected.json, server_error.json,
// library_movie_held.json and queue_none.json, which follow the source
// because the live instance can't produce them safely (or holds no movie
// yet), and *_later.json, which advance a recording to show progress.
type route struct {
	status  int
	fixture string
	delay   time.Duration
}

func ok(fixture string) route { return route{status: http.StatusOK, fixture: fixture} }

// transfer is one record of MoviePilot's transfer (整理) history.
type transfer struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	MediaSource string `json:"media_source"`
	MediaID     string `json:"media_id"`
	Seasons     string `json:"seasons"`
	Episodes    string `json:"episodes"`
	Image       string `json:"image"`
	Year        string `json:"year"`
	Status      bool   `json:"status"`
	Src         string `json:"src,omitempty"`
	Hash        string `json:"download_hash,omitempty"`
}

// fakeMoviePilot serves canned routes plus the live transfer history a
// scenario drives. The notifier polls the history and subscription status
// continuously, so those calls stay out of the transcript; the notices they
// cause are recorded on the Telegram side instead.
type fakeMoviePilot struct {
	*httptest.Server
	t      *testing.T
	tr     *transcript
	routes map[string]route

	mu         sync.Mutex
	transfers  []transfer // newest first, like MoviePilot
	polled     chan struct{}
	readers    []reader
	created    int  // subscriptions created so far
	added      int  // downloads added so far
	lagging    bool // the media server has not scanned new transfers yet
	scanned    int  // transfers the media server shows while lagging
	searches   map[string]string
	recognized map[string]string
	deleted    map[string]bool // downloads removed from the downloader, by hash
	states     map[int]string  // subscription states set, by id
}

// reader waits for the notifier to finish a poll that read the transfer
// history up to id.
type reader struct {
	id   int
	seen bool
	done chan struct{}
}

func newFakeMoviePilot(t *testing.T, tr *transcript, routes map[string]route) *fakeMoviePilot {
	all := maps.Clone(idleServer)
	maps.Copy(all, routes)
	f := &fakeMoviePilot{t: t, tr: tr, routes: all, polled: make(chan struct{}), deleted: map[string]bool{}, states: map[int]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	return f
}

// add appends records to the transfer history; the returned channel closes
// once a poll that read them has finished.
func (f *fakeMoviePilot) add(records ...transfer) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range records {
		r.ID = len(f.transfers) + 1
		r.Status = true
		f.transfers = append([]transfer{r}, f.transfers...)
	}
	rd := reader{id: len(f.transfers), done: make(chan struct{})}
	f.readers = append(f.readers, rd)
	return rd.done
}

// markRead is called on every history read with the newest id served; f.mu
// held. Polls run one after another and a poll's handful of records fits on
// one page, so the read after the one that saw a reader's records means
// that poll, including its notices, is done.
func (f *fakeMoviePilot) markRead(newest int) {
	waiting := f.readers[:0]
	for _, rd := range f.readers {
		if rd.seen {
			close(rd.done)
			continue
		}
		rd.seen = rd.id <= newest
		waiting = append(waiting, rd)
	}
	f.readers = waiting
}

func (f *fakeMoviePilot) serve(w http.ResponseWriter, r *http.Request) {
	if !f.wait(r) {
		return
	}
	isTransfers := r.Method == http.MethodGet && r.URL.Path == transferPath
	if isTransfers {
		f.markPolled()
	}
	if !isPolled(r) {
		f.record(r)
	}
	if r.Header.Get("X-API-KEY") != mpAPIKey {
		writeEnvelope(w, http.StatusUnauthorized, `{"success":false,"message":"apikey 校验不通过","data":null}`)
		return
	}
	switch {
	case isTransfers:
		f.serveTransfers(w, r)
	case r.Method == http.MethodGet && isSubscriptionByID(r.URL.Path):
		f.serveSubscription(w, r)
	case f.servesLibrary(w, r):
	case f.deletesDownload(w, r):
	case f.setsSubscriptionState(w, r):
	default:
		f.serveRoute(w, r)
	}
}

func (f *fakeMoviePilot) wait(r *http.Request) bool {
	f.mu.Lock()
	d := f.routes[r.Method+" "+r.URL.Path].delay
	f.mu.Unlock()
	if d == 0 {
		return true
	}
	select {
	case <-time.After(d):
		return true
	case <-r.Context().Done():
		return false
	}
}

// isPolled reports whether r is one of the reads repeated in the
// background, which the transcript leaves out.
func isPolled(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return polledPaths[r.Method+" "+r.URL.Path]
	}
	return r.URL.Path == transferPath || isSubscriptionByID(r.URL.Path) || polledPaths[r.Method+" "+r.URL.Path]
}

// markPolled signals the notifier's first look at the transfer history.
func (f *fakeMoviePilot) markPolled() {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case <-f.polled:
	default:
		close(f.polled)
	}
}

func (f *fakeMoviePilot) record(r *http.Request) {
	path, _ := url.PathUnescape(r.URL.EscapedPath())
	query, _ := url.QueryUnescape(r.URL.RawQuery)
	head := "-> MoviePilot " + r.Method + " " + path
	if query != "" {
		head += "?" + query
	}
	body, _ := io.ReadAll(r.Body)
	if r.Method+" "+r.URL.Path == downloadPath {
		f.tr.add(head, downloadSummary(body))
		return
	}
	if len(body) > 0 {
		f.tr.add(head, string(body))
		return
	}
	f.tr.add(head)
}

// downloadSummary records a download request by the release it names and
// whether it carries the search result back with its site credentials, as
// MoviePilot needs to fetch the torrent; the credentials stay out of the
// transcript.
func downloadSummary(body []byte) string {
	var sent struct {
		Media   *struct{ Title string } `json:"media_in"`
		Torrent *struct {
			Title     string `json:"title"`
			Cookie    string `json:"site_cookie"`
			Enclosure string `json:"enclosure"`
		} `json:"torrent_in"`
	}
	if json.Unmarshal(body, &sent) != nil || sent.Media == nil || sent.Torrent == nil {
		return "malformed download request: " + string(body)
	}
	handed := "without the search result's site credentials"
	if sent.Torrent.Cookie == siteCookie && sent.Torrent.Enclosure != "" {
		handed = "with the search result's site credentials"
	}
	return fmt.Sprintf("media_in %s, torrent_in %s %s", sent.Media.Title, sent.Torrent.Title, handed)
}

// collectionSearch is the route key of a search for collections named
// title; searches share one path, so collections are told apart by name.
func collectionSearch(title string) string {
	return searchPath + "?type=collection&title=" + title
}

// setRoute changes what MoviePilot answers from now on, e.g. a download
// making progress.
func (f *fakeMoviePilot) setRoute(key string, rt route) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[key] = rt
}

func (f *fakeMoviePilot) serveRoute(w http.ResponseWriter, r *http.Request) {
	path, _ := url.PathUnescape(r.URL.EscapedPath())
	key, title := r.Method+" "+path, r.URL.Query().Get("title")
	f.mu.Lock()
	rt, found := f.routes[key]
	if fixture, ok := f.searches[title]; ok && key == searchPath {
		rt, found = route{status: http.StatusOK, fixture: fixture}, true
	}
	if key == recognizePath {
		// MoviePilot answers a file it cannot recognize with no meta_info.
		rt, found = ok(cmp.Or(f.recognized[title], "recognize_none.json")), true
	}
	if key == searchPath && r.URL.Query().Get("type") == "collection" {
		// MoviePilot answers a name matching no collection with an empty list.
		rt, found = f.routes[collectionSearch(title)], true
		rt = cmp.Or(rt, ok("empty.json"))
	}
	f.mu.Unlock()
	if !found {
		writeEnvelope(w, http.StatusNotFound, `{"success":false,"message":"Not Found","data":null}`)
		return
	}
	data, err := os.ReadFile(filepath.Join("testdata", "moviepilot", rt.fixture))
	if err != nil {
		f.t.Errorf("fixture: %v", err)
	}
	body := string(data)
	if rt.fixture == createdFixture {
		body = f.numbered(body)
	}
	if key == downloadsPath {
		body = f.undeleted(body)
	}
	if key == subsPath && rt.status == http.StatusOK {
		body = f.restated(body)
	}
	if key == downloadPath && rt.status == http.StatusOK {
		body = f.hashed(body)
	}
	writeEnvelope(w, rt.status, body)
}

// numbered gives each created subscription its own id, as MoviePilot does;
// the recording carries the id of the one live subscription.
func (f *fakeMoviePilot) numbered(body string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created++
	return strings.Replace(body, createdID, fmt.Sprintf(`"id": %d`, f.created), 1)
}

// hashed gives each added download its own torrent hash, as MoviePilot
// does: the first keeps addedHash, later ones end in their number.
func (f *fakeMoviePilot) hashed(body string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added++
	if f.added == 1 {
		return body
	}
	return strings.Replace(body, addedHash, addedHashN(f.added), 1)
}

// addedHashN is the hash of the nth download added in a scenario.
func addedHashN(n int) string {
	suffix := fmt.Sprintf("%02d", n)
	return addedHash[:len(addedHash)-len(suffix)] + suffix
}

// serveTransfers pages the history the way MoviePilot does (newest first).
func (f *fakeMoviePilot) serveTransfers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	f.mu.Lock()
	from := min(max(page-1, 0)*count, len(f.transfers))
	list := slices.Clone(f.transfers[from:min(from+count, len(f.transfers))])
	total := len(f.transfers)
	newest := 0
	if len(list) > 0 {
		newest = list[0].ID
	}
	f.markRead(newest)
	f.mu.Unlock()
	data, _ := json.Marshal(map[string]any{"list": list, "total": total})
	writeEnvelope(w, http.StatusOK, `{"success":true,"message":"","data":`+string(data)+`}`)
}

// serveSubscription answers GET /api/v1/subscribe/{id}: every subscription
// a scenario creates stays active.
func (f *fakeMoviePilot) serveSubscription(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, subscriptionPath)
	writeEnvelope(w, http.StatusOK, `{"success":true,"message":"","data":{"id":`+id+`}}`)
}

func isSubscriptionByID(path string) bool {
	_, err := strconv.Atoi(strings.TrimPrefix(path, subscriptionPath))
	return strings.HasPrefix(path, subscriptionPath) && err == nil
}

func writeEnvelope(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// deletesDownload answers DELETE /api/v1/download/{hash} like MoviePilot:
// success when the downloader had the download, which then leaves the
// download list. A scenario route for the exact path overrides it.
func (f *fakeMoviePilot) deletesDownload(w http.ResponseWriter, r *http.Request) bool {
	hash, found := strings.CutPrefix(r.URL.Path, downloadTaskPrefix)
	f.mu.Lock()
	_, routed := f.routes[r.Method+" "+r.URL.Path]
	listed := f.routes[downloadsPath].fixture
	f.mu.Unlock()
	if r.Method != http.MethodDelete || !found || routed {
		return false
	}
	data, _ := os.ReadFile(filepath.Join("testdata", "moviepilot", listed))
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleted[hash] || !strings.Contains(string(data), `"hash": "`+hash+`"`) {
		writeEnvelope(w, http.StatusOK, `{"success":false,"message":null,"data":null}`)
		return true
	}
	f.deleted[hash] = true
	writeEnvelope(w, http.StatusOK, `{"success":true,"message":null,"data":null}`)
	return true
}

// undeleted leaves the deleted downloads out of a download list.
func (f *fakeMoviePilot) undeleted(body string) string {
	var env struct {
		Success bool             `json:"success"`
		Message string           `json:"message"`
		Data    []map[string]any `json:"data"`
	}
	if json.Unmarshal([]byte(body), &env) != nil {
		return body
	}
	f.mu.Lock()
	env.Data = slices.DeleteFunc(env.Data, func(d map[string]any) bool { return f.deleted[fmt.Sprint(d["hash"])] })
	f.mu.Unlock()
	out, _ := json.Marshal(env)
	return string(out)
}
