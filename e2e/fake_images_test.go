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
// other, as the real host does. A poster is a tiny JPEG derived from its
// path, so uploads are stable across runs.
type fakeImages struct {
	*httptest.Server

	mu      sync.Mutex
	missing map[string]bool // paths answering 404
}

func newFakeImages() *fakeImages {
	f := &fakeImages{missing: map[string]bool{}}
	f.Server = httptest.NewServer(f)
	return f
}

// setMissing makes the poster at path answer 404, like a deleted image.
func (f *fakeImages) setMissing(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.missing[path] = true
}

func (f *fakeImages) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	missing := f.missing[r.URL.Path]
	f.mu.Unlock()
	switch {
	case !strings.HasSuffix(r.Header.Get(forwardedHost), ".doubanio.com"):
		http.Error(w, "only Douban's image host is faked", http.StatusBadGateway)
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
