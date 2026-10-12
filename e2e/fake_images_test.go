package e2e

import (
	"bytes"
	"crypto/sha256"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

const (
	doubanReferer = "https://movie.douban.com/"
	posterWidth   = 2
	posterHeight  = 3
	// forwardedHost carries the host a poster URL named to the fake.
	forwardedHost = "X-Forwarded-Host"
)

// fakeImages is Douban's image host (img*.doubanio.com): it serves posters
// only to requests that name Douban as referer and answers 418 to any
// other, and refuses Go's default User-Agent with 403, as the real hosts
// do. A poster is a tiny JPEG derived from its path, so uploads are stable
// across runs.
type fakeImages struct {
	*httptest.Server
	t *testing.T

	mu      sync.Mutex
	missing map[string]bool          // paths answering 404
	hits    map[string]int           // requests received per path
	reached map[string]chan struct{} // closed at a path's first request
	held    map[string]chan struct{} // paths whose answer waits for release
}

func newFakeImages(t *testing.T) *fakeImages {
	f := &fakeImages{t: t, missing: map[string]bool{}, hits: map[string]int{},
		reached: map[string]chan struct{}{}, held: map[string]chan struct{}{}}
	f.Server = httptest.NewServer(f)
	return f
}

// setMissing makes the poster at path answer 404, like a deleted image.
func (f *fakeImages) setMissing(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.missing[path] = true
}

// downloads is how many requests the poster at path has received.
func (f *fakeImages) downloads(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

// firstRequest closes when the poster at path receives its first request.
func (f *fakeImages) firstRequest(path string) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reachedLocked(path)
}

func (f *fakeImages) reachedLocked(path string) chan struct{} {
	if f.reached[path] == nil {
		f.reached[path] = make(chan struct{})
	}
	return f.reached[path]
}

// hold keeps answering the poster at path until the returned release is
// called, or until the client gives up.
func (f *fakeImages) hold(path string) (release func()) {
	gate := make(chan struct{})
	f.mu.Lock()
	f.held[path] = gate
	f.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { close(gate) }) }
}

// arrive counts a request and wakes whoever waits for the path's first.
func (f *fakeImages) arrive(path string) (missing bool, gate chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hits[path]++; f.hits[path] == 1 {
		close(f.reachedLocked(path))
	}
	return f.missing[path], f.held[path]
}

func (f *fakeImages) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	assertNoResourceCredentials(f.t, r)
	missing, gate := f.arrive(r.URL.Path)
	if gate != nil {
		select {
		case <-gate:
		case <-r.Context().Done():
			return
		}
	}
	switch {
	case !strings.HasSuffix(r.Header.Get(forwardedHost), ".doubanio.com"):
		http.Error(w, "only Douban's image host is faked", http.StatusBadGateway)
	case strings.HasPrefix(r.UserAgent(), "Go-http-client/"):
		http.Error(w, "403 Forbidden", http.StatusForbidden)
	case r.Header.Get("Referer") != doubanReferer:
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(http.StatusTeapot)
	case missing:
		http.NotFound(w, r)
	default:
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(posterJPEG(r.URL.Path))
	}
}

func posterJPEG(path string) []byte {
	sum := sha256.Sum256([]byte(path))
	img := image.NewRGBA(image.Rect(0, 0, posterWidth, posterHeight))
	for x := range posterWidth {
		for y := range posterHeight {
			img.Set(x, y, color.RGBA{R: sum[0], G: sum[1], B: sum[2], A: 0xff})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

// client sends every request to the fake, keeping its path and naming the
// original host, so posters keep their recorded URLs.
func (f *fakeImages) client() *http.Client {
	target, _ := url.Parse(f.URL)
	return &http.Client{Transport: redirect{target: target}}
}

type redirect struct{ target *url.URL }

func (rt redirect) RoundTrip(r *http.Request) (*http.Response, error) {
	out := r.Clone(r.Context())
	out.Header.Set(forwardedHost, r.URL.Hostname())
	out.URL.Scheme, out.URL.Host, out.Host = rt.target.Scheme, rt.target.Host, rt.target.Host
	return http.DefaultTransport.RoundTrip(out)
}
