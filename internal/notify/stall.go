package notify

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// mark is a download's progress and when it last changed.
type mark struct {
	progress float64
	since    time.Time
}

// stall is a watch's download that has made no progress for Options.Stall.
type stall struct {
	watch    watch
	download flow.Download
}

// checkStalls tells requesters once about each of their downloads that has
// not moved for Options.Stall. MoviePilot closes a subscription as soon as
// its downloads are added, so a dead torrent would otherwise end in
// silence. Paused downloads were stopped on purpose and are not stuck.
func (n *Notifier) checkStalls(ctx context.Context) {
	if n.opts.Stall <= 0 || len(n.snapshot().Watches) == 0 {
		return
	}
	downloads, err := n.opts.Feed.Downloads(ctx)
	if err != nil {
		if ctx.Err() == nil {
			n.opts.Log.Warn("download progress unavailable", "err", err)
		}
		return
	}
	stuck := n.unmoved(downloads, n.opts.Now())
	var stalls []stall
	err = n.update(func(st state) state {
		next, out := withStalls(st, stuck)
		stalls = out
		return next
	})
	if err != nil {
		n.opts.Log.Warn("stalled downloads not remembered", "err", err)
		return
	}
	for _, s := range stalls {
		n.tell(ctx, s.watch, flow.Notice{Text: stallText(s, n.opts.Stall), Image: s.download.Image})
	}
}

// unmoved updates the progress marks and lists the downloads that have not
// moved for Options.Stall; marks of finished, removed or paused downloads go.
func (n *Notifier) unmoved(downloads []flow.Download, now time.Time) []flow.Download {
	live := map[string]bool{}
	var out []flow.Download
	for _, d := range downloads {
		if d.Paused {
			continue
		}
		live[d.ID] = true
		m, known := n.marks[d.ID]
		if !known || d.Progress != m.progress {
			n.marks[d.ID] = mark{progress: d.Progress, since: now}
			continue
		}
		if now.Sub(m.since) >= n.opts.Stall {
			out = append(out, d)
		}
	}
	for id := range n.marks {
		if !live[id] {
			delete(n.marks, id)
		}
	}
	return out
}

// withStalls records the stuck downloads not yet told about against the
// watches waiting for them, and returns those to tell.
func withStalls(st state, stuck []flow.Download) (state, []stall) {
	next := st.emptied()
	var out []stall
	for _, w := range st.Watches {
		for _, d := range stuck {
			if !w.awaits(d) || slices.Contains(w.Stalled, d.ID) {
				continue
			}
			w.Stalled = append(slices.Clone(w.Stalled), d.ID)
			out = append(out, stall{watch: w, download: d})
		}
		next.Watches = append(next.Watches, w)
	}
	return next, out
}

// awaits reports whether d downloads something w still waits for: its
// media and season, and (when known) an episode that has not arrived; a
// download's watch waits for that download alone.
func (w watch) awaits(d flow.Download) bool {
	if w.Download != "" {
		return d.ID == w.Download
	}
	if d.Source != w.Source || d.MediaID != w.MediaID {
		return false
	}
	if w.Season == nil {
		return d.Season == nil
	}
	if d.Season == nil || *d.Season != *w.Season {
		return false
	}
	return len(d.Episodes) == 0 || len(w.fresh(d.Episodes)) > 0
}

// stallText is e.g. "⚠️ 下载好像卡住了" over "你想看的《颂乐人偶》第 1 季
// E01–E02 已经 6 小时没有进展啦，一直停在 0%。".
func stallText(s stall, after time.Duration) flow.Text {
	name := fmt.Sprintf("《%s》", s.watch.Title)
	if s.watch.Season != nil {
		name += fmt.Sprintf("第 %d 季", *s.watch.Season)
	}
	if eps := flow.EpisodeRanges(s.download.Episodes); eps != "" {
		name += " " + eps
	}
	still := fmt.Sprintf(" 已经 %s没有进展啦，一直停在 %.0f%%。", wordDuration(after), s.download.Progress)
	return flow.Lines(
		flow.Heading(flow.Plain("⚠️ 下载好像卡住了")),
		flow.Line(flow.Plain("你想看的"), flow.Strong(name), flow.Plain(still)),
		flow.Line(flow.Plain("可能是没人做种了，去 /tasks 看看，或者到 MoviePilot 里换个资源吧 (｡•́︿•̀｡)")),
	)
}

// wordDuration is e.g. "6 小时" or "30 分钟".
func wordDuration(d time.Duration) string {
	if d >= time.Hour && d%time.Hour == 0 {
		return fmt.Sprintf("%d 小时", int(d.Hours()))
	}
	return fmt.Sprintf("%d 分钟", int(d.Minutes()))
}
