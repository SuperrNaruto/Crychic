package notify

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const (
	daysInWeek = 7
	// maxDigestItems bounds the titles one digest lists.
	maxDigestItems = 30
)

// Schedule is a weekly moment: Day at At past midnight in Zone. A nil Zone
// is no schedule.
type Schedule struct {
	Day  time.Weekday
	At   time.Duration
	Zone *time.Location
}

// latest is the schedule's most recent moment at or before now.
func (s Schedule) latest(now time.Time) time.Time {
	local := now.In(s.Zone)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, s.Zone)
	back := (int(local.Weekday()) - int(s.Day) + daysInWeek) % daysInWeek
	slot := midnight.AddDate(0, 0, -back).Add(s.At)
	if slot.After(now) {
		slot = slot.AddDate(0, 0, -daysInWeek)
	}
	return slot
}

// digestEntry is one transferred record for the weekly digest.
type digestEntry struct {
	Source   string    `json:"source"`
	MediaID  string    `json:"media_id"`
	Title    string    `json:"title"`
	Year     string    `json:"year,omitempty"`
	Season   *int      `json:"season,omitempty"`
	Episodes []int     `json:"episodes,omitempty"`
	At       time.Time `json:"at"`
}

// withDigest remembers every transfer for the next digest, requested or not.
func withDigest(st state, transfers []Transfer, now time.Time) state {
	if len(transfers) == 0 {
		return st
	}
	next := st
	next.Digest = slices.Clone(st.Digest)
	for _, t := range transfers {
		next.Digest = append(next.Digest, digestEntry{
			Source: t.Source, MediaID: t.MediaID, Title: t.Title, Year: t.Year,
			Season: t.Season, Episodes: t.Episodes, At: now,
		})
	}
	return next
}

// checkDigest sends the weekly digest once its moment has passed since the
// last one. The first check only marks when digests began, so a newly
// enabled digest waits for its first moment. State is saved before
// sending: a crash can lose a digest, never repeat one.
func (n *Notifier) checkDigest(ctx context.Context) error {
	if n.opts.Digest.Zone == nil {
		return nil
	}
	now := n.opts.Now()
	slot := n.opts.Digest.latest(now)
	var entries []digestEntry
	err := n.update(func(st state) state {
		switch {
		case st.LastDigest == nil:
			st.LastDigest = &now
		case st.LastDigest.Before(slot):
			entries = st.Digest
			st.LastDigest, st.Digest = &now, nil
		}
		return st
	})
	if err != nil || len(entries) == 0 {
		return err
	}
	text := digestText(mergeDigest(entries), n.latestItems(ctx))
	recipients := n.opts.DigestTo
	if n.opts.Destination != "" {
		recipients = []flow.Actor{{Address: n.opts.Destination}}
	}
	for _, to := range recipients {
		if err := n.opts.Sender.Notify(ctx, flow.Notice{To: to, Text: text}); err != nil {
			n.opts.Log.Error("digest not delivered", "user", to.UserID, "err", err)
		}
	}
	return nil
}

// latestItems are the media servers' newest items, to link titles to;
// none when they cannot be read.
func (n *Notifier) latestItems(ctx context.Context) []flow.LibraryItem {
	items, err := n.opts.Feed.Latest(ctx)
	if err != nil {
		n.opts.Log.Warn("latest items unavailable", "err", err)
	}
	return items
}

// mergeDigest joins the entries of one title and season, in order of first
// arrival, their episodes sorted once each.
func mergeDigest(entries []digestEntry) []digestEntry {
	var merged []digestEntry
	for _, e := range entries {
		i := slices.IndexFunc(merged, func(m digestEntry) bool {
			return m.Source == e.Source && m.MediaID == e.MediaID && seasonOf(m) == seasonOf(e)
		})
		if i < 0 {
			e.Episodes = slices.Clone(e.Episodes)
			merged = append(merged, e)
			continue
		}
		merged[i].Episodes = append(merged[i].Episodes, e.Episodes...)
	}
	for i := range merged {
		slices.Sort(merged[i].Episodes)
		merged[i].Episodes = slices.Compact(merged[i].Episodes)
	}
	return merged
}

// seasonOf is an entry's season, -1 for a movie.
func seasonOf(e digestEntry) int {
	if e.Season == nil {
		return -1
	}
	return *e.Season
}

// digestText lists what arrived, each title linked to where it can be
// watched when the media server's newest items show it.
func digestText(merged []digestEntry, items []flow.LibraryItem) flow.Text {
	text := flow.Lines(
		flow.Heading(flow.Plain("📥 本周入库")),
		flow.Line(flow.Plain(fmt.Sprintf("这周有 %d 部新到媒体库啦 ヾ(≧▽≦*)o", len(merged)))),
	)
	for i, e := range merged[:min(len(merged), maxDigestItems)] {
		name := flow.Strong(fmt.Sprintf("《%s》", e.Title))
		if link := linkOf(e, items); link != "" {
			name = flow.Linked(name, link)
		}
		text = append(text, flow.Block{Kind: flow.Item, Number: i + 1, Spans: []flow.Span{name}, Facts: digestFacts(e)})
	}
	if more := len(merged) - maxDigestItems; more > 0 {
		text = append(text, flow.Remark(fmt.Sprintf("还有 %d 部没列出", more)))
	}
	return append(text, flow.Remark("统计上次汇总以来 MoviePilot 整理进媒体库的内容"))
}

// digestFacts is e.g. "第 2 季 · E01–E06", or 电影.
func digestFacts(e digestEntry) string {
	if e.Season == nil {
		return "电影"
	}
	facts := fmt.Sprintf("第 %d 季", *e.Season)
	if eps := flow.EpisodeRanges(e.Episodes); eps != "" {
		facts += " · " + eps
	}
	return facts
}

func linkOf(e digestEntry, items []flow.LibraryItem) string {
	for _, it := range items {
		if it.Title == e.Title && (e.Year == "" || it.Year == e.Year) {
			return it.Link
		}
	}
	return ""
}
