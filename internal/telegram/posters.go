package telegram

import (
	"bytes"
	"container/list"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

const (
	// posterBudget bounds downloading a reply's posters; they are cosmetic.
	posterBudget = 5 * time.Second
	// maxPosterBytes keeps a poster well within Telegram's photo upload limit.
	maxPosterBytes = 5 << 20
	// posterCacheSize is how many uploaded posters keep their file_id.
	posterCacheSize = 1000
	// posterFetches is how many posters download at once.
	posterFetches = 4
)

// hotlinked are image hosts that refuse requests without their site as
// referer (Douban answers 418), so Telegram cannot fetch their posters
// itself; the bot downloads and uploads them instead.
var hotlinked = map[string]string{".doubanio.com": "https://movie.douban.com/"}

func refererFor(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	for suffix, referer := range hotlinked {
		if strings.HasSuffix(u.Hostname(), suffix) {
			return referer, true
		}
	}
	return "", false
}

// posters uploads hotlinked posters and remembers the file_id Telegram
// gives each one, so a poster is downloaded and uploaded only once while
// it stays among the most recently used.
type posters struct {
	client *http.Client
	log    *slog.Logger

	mu    sync.Mutex
	ids   map[string]*list.Element // poster URL → element holding a cached
	order *list.List               // most recently used first
}

type cached struct{ url, fileID string }

func newPosters(client *http.Client, log *slog.Logger) *posters {
	return &posters{client: client, log: log, ids: map[string]*list.Element{}, order: list.New()}
}

// outgoing is a reply ready to send: hotlinked posters are replaced by
// media references, uploads carry the downloaded bytes, and learn maps a
// poster's position on the message to the URL whose file_id it will carry.
type outgoing struct {
	reply   flow.Reply
	uploads []upload
	reuses  []reuse
	learn   map[int]string
}

type upload struct {
	id, name string
	data     []byte
}

type reuse struct{ id, url, fileID string }

// prepare swaps hotlinked posters for uploads, or for the file_id of an
// earlier upload; a poster that cannot be downloaded is left out.
func (p *posters) prepare(ctx context.Context, reply flow.Reply) outgoing {
	out := outgoing{reply: reply, learn: map[int]string{}}
	urls := shownPosters(reply)
	fetched := p.fetchAll(ctx, p.missing(urls))
	var kept []string
	for i, raw := range urls {
		if _, ok := refererFor(raw); !ok {
			kept = append(kept, raw)
			continue
		}
		id := fmt.Sprintf("p%d", i)
		if fileID, ok := p.lookup(raw); ok {
			out.reuses = append(out.reuses, reuse{id: id, url: raw, fileID: fileID})
		} else if data, ok := fetched[raw]; ok {
			out.uploads = append(out.uploads, upload{id: id, name: id + "-" + path.Base(raw), data: data})
			out.learn[len(kept)] = raw
		} else {
			continue
		}
		kept = append(kept, "tg://photo?id="+id)
	}
	out.reply = withPosters(reply, kept)
	return out
}

// shownPosters are the posters a reply shows, in message order.
func shownPosters(reply flow.Reply) []string {
	if reply.Image != "" {
		return []string{reply.Image}
	}
	return reply.Gallery
}

func withPosters(reply flow.Reply, urls []string) flow.Reply {
	if reply.Image != "" {
		reply.Image = ""
		if len(urls) > 0 {
			reply.Image = urls[0]
		}
		return reply
	}
	reply.Gallery = urls
	return reply
}

// media lists the message's media references for InputRichMessage.Media.
func (o outgoing) media() []models.InputRichMessageMedia {
	var media []models.InputRichMessageMedia
	for _, u := range o.uploads {
		media = append(media, models.InputRichMessageMedia{ID: u.id, Media: &models.InputMediaPhoto{
			Media: "attach://" + u.name, MediaAttachment: bytes.NewReader(u.data),
		}})
	}
	for _, r := range o.reuses {
		media = append(media, models.InputRichMessageMedia{ID: r.id, Media: &models.InputMediaPhoto{Media: r.fileID}})
	}
	return media
}

// missing are the hotlinked posters without a cached file_id.
func (p *posters) missing(urls []string) []string {
	var out []string
	for _, raw := range urls {
		if _, ok := refererFor(raw); !ok {
			continue
		}
		if _, ok := p.lookup(raw); !ok {
			out = append(out, raw)
		}
	}
	return out
}

func (p *posters) fetchAll(ctx context.Context, urls []string) map[string][]byte {
	got := map[string][]byte{}
	if len(urls) == 0 {
		return got
	}
	ctx, cancel := context.WithTimeout(ctx, posterBudget)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, posterFetches)
	for _, raw := range urls {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			data, err := p.fetch(ctx, raw)
			if err != nil {
				p.log.Warn("poster download failed", "image", raw, "err", err)
				return
			}
			mu.Lock()
			got[raw] = data
			mu.Unlock()
		})
	}
	wg.Wait()
	return got
}

func (p *posters) fetch(ctx context.Context, raw string) ([]byte, error) {
	referer, _ := refererFor(raw)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", referer)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	if kind := resp.Header.Get("Content-Type"); !strings.HasPrefix(kind, "image/") {
		return nil, fmt.Errorf("content type %q", kind)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPosterBytes+1))
	if err == nil && len(data) > maxPosterBytes {
		err = fmt.Errorf("larger than %d bytes", maxPosterBytes)
	}
	return data, err
}

// sent remembers the file_ids of uploaded posters from Telegram's copy of
// the message; a refused message drops the file_ids it reused, in case
// one of them is what Telegram refused.
func (p *posters) sent(o outgoing, msg *models.Message, err error) {
	if err != nil {
		for _, r := range o.reuses {
			p.forget(r.url)
		}
		return
	}
	if len(o.learn) == 0 || msg == nil || msg.RichMessage == nil {
		return
	}
	ids := headerPhotos(msg.RichMessage.Blocks)
	for at, raw := range o.learn {
		if at < len(ids) {
			p.remember(raw, ids[at])
		}
	}
}

// headerPhotos are the file_ids of the poster or slideshow topping a
// message, in order, each in its largest size.
func headerPhotos(blocks []models.RichBlock) []string {
	if len(blocks) == 0 {
		return nil
	}
	top := []models.RichBlock{blocks[0]}
	if s := blocks[0].RichBlockSlideshow; s != nil {
		top = s.Blocks
	}
	var ids []string
	for _, b := range top {
		if b.RichBlockPhoto == nil || len(b.RichBlockPhoto.Photo) == 0 {
			return ids
		}
		sizes := b.RichBlockPhoto.Photo
		ids = append(ids, sizes[len(sizes)-1].FileID)
	}
	return ids
}

func (p *posters) lookup(raw string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	el, ok := p.ids[raw]
	if !ok {
		return "", false
	}
	p.order.MoveToFront(el)
	return el.Value.(cached).fileID, true
}

func (p *posters) remember(raw, fileID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if el, ok := p.ids[raw]; ok {
		el.Value = cached{url: raw, fileID: fileID}
		p.order.MoveToFront(el)
		return
	}
	p.ids[raw] = p.order.PushFront(cached{url: raw, fileID: fileID})
	if p.order.Len() > posterCacheSize {
		oldest := p.order.Back()
		p.order.Remove(oldest)
		delete(p.ids, oldest.Value.(cached).url)
	}
}

func (p *posters) forget(raw string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if el, ok := p.ids[raw]; ok {
		p.order.Remove(el)
		delete(p.ids, raw)
	}
}
