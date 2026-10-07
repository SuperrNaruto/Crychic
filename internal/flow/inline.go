package flow

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	// inlineResults bounds the media offered for one inline query.
	inlineResults = 10
	// inlineMinRunes is the shortest query worth searching for.
	inlineMinRunes = 2
	refPrefix      = "m"
	refSeparator   = "_"
	refParts       = 4
)

// refSources and refKinds keep media refs short; refID keeps them safe in
// a deep link, which only allows letters, digits, _ and -.
var (
	refSources = map[string]string{"themoviedb": "t", "douban": "d", "bangumi": "b"}
	refKinds   = map[Kind]string{Movie: "mv", TV: "tv"}
	refID      = regexp.MustCompile(`^[A-Za-z0-9-]{1,40}$`)
)

// InlineResult is one media offered for an inline query: listed in the
// platform's picker and shared as a short card that opens it in the bot.
type InlineResult struct {
	Ref         string // MediaRef of the media
	Title       string
	Description string // e.g. 2021 · 电影 · ⭐ 7.8
	Thumb       string // poster URL, "" when none
	Link        string // the media's page on its metadata site, "" when unknown
}

// MediaRef names media in a deep link, e.g. m_tv_t_224207; "" when it
// cannot be named.
func MediaRef(m Media) string {
	source, kind := refSources[m.Source], refKinds[m.Kind]
	if source == "" || kind == "" || !refID.MatchString(m.ID) {
		return ""
	}
	return strings.Join([]string{refPrefix, kind, source, m.ID}, refSeparator)
}

// parseMediaRef reads a MediaRef back into the media's identity.
func parseMediaRef(ref string) (Media, bool) {
	parts := strings.SplitN(ref, refSeparator, refParts)
	if len(parts) != refParts || parts[0] != refPrefix || !refID.MatchString(parts[3]) {
		return Media{}, false
	}
	m := Media{ID: parts[3]}
	for k, code := range refKinds {
		if code == parts[1] {
			m.Kind = k
		}
	}
	for source, code := range refSources {
		if code == parts[2] {
			m.Source = source
		}
	}
	return m, m.Kind != 0 && m.Source != ""
}

// Find searches media for an inline query; a query too short finds
// nothing without asking the backend.
func (e *Engine) Find(ctx context.Context, term string) ([]InlineResult, error) {
	term = strings.TrimSpace(term)
	if utf8.RuneCountInString(term) < inlineMinRunes {
		return nil, nil
	}
	found, err := e.backend.Search(ctx, term)
	if err != nil {
		return nil, err
	}
	var results []InlineResult
	seen := map[string]bool{}
	for _, m := range found {
		ref := MediaRef(m)
		if ref == "" || seen[ref] || len(results) == inlineResults {
			continue
		}
		seen[ref] = true
		results = append(results, InlineResult{
			Ref: ref, Title: m.Title, Description: joinNonEmpty(" · ", m.Year, m.Kind.String(), rating(m.Rating)),
			Thumb: m.PosterURL, Link: m.Link,
		})
	}
	return results, nil
}

// Open shows the card of the media a MediaRef names, as if it were the
// one result of a search; a ref it cannot read opens the home menu.
func (e *Engine) Open(ctx context.Context, actor Actor, ref string) Reply {
	identity, ok := parseMediaRef(ref)
	if !ok {
		return e.Home(ctx, actor)
	}
	media, err := e.backend.Lookup(ctx, identity)
	if err != nil {
		return titled("🔍 打开失败", e.failure("open media", err))
	}
	sess := e.store.create(actor, nil)
	sess.results = []Media{media}
	return e.shown(sess.id, e.pickMedia(ctx, sess, 0))
}
