package notify

import (
	"fmt"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// arrivedHeading opens every arrival notice.
var arrivedHeading = flow.Heading(flow.Plain("📥 入库啦"))

// arrivalText is the notice for one delivery, e.g. "📥 入库啦" over
// "你想看的《碧蓝之海》第 3 季 E01–E03、E05 到家啦！".
func arrivalText(d delivery) flow.Text {
	w := d.watch
	name := fmt.Sprintf("《%s》", w.Title)
	if w.Season == nil {
		text := flow.Lines(arrivedHeading, flow.Line(flow.Plain("你想看的"), flow.Strong(name), flow.Plain("到家啦，快去看吧！(ﾉ>ω<)ﾉ")))
		return withWatchLink(withQualities(text, d), d)
	}
	name += fmt.Sprintf("第 %d 季", *w.Season)
	text := flow.Lines(arrivedHeading, flow.Line(
		flow.Plain("你想看的"), flow.Strong(name), flow.Plain(" "+flow.EpisodeRanges(d.episodes)+" 到家啦！"),
	))
	text = withQualities(text, d)
	if d.complete {
		text = append(text, flow.Line(flow.Plain(completeWords(d)+" ヾ(≧▽≦*)o")))
	}
	return withWatchLink(text, d)
}

// completeWords says everything wanted arrived: every episode the
// downloads told bring, which may be a single one, or else the whole
// requested season, which a download of it brings as well.
func completeWords(d delivery) string {
	if d.downloads && d.held {
		return "加上媒体库已有的，这一季的剧集全部到齐啦"
	}
	if d.downloads {
		return "这次下载的剧集全部到齐啦"
	}
	return "这一季你要的剧集全部到齐啦"
}

// withQualities adds a line of small print per release quality, e.g.
// "1080p · BluRay · x265 · FLAC 2.0 · FROGE".
func withQualities(text flow.Text, d delivery) flow.Text {
	for _, q := range d.qualities {
		text = append(text, flow.Small(flow.Plain(q)))
	}
	return text
}

// withWatchLink adds a button to where to watch, e.g. "在 Emby 中观看".
func withWatchLink(text flow.Text, d delivery) flow.Text {
	if d.watchAt.Link == "" {
		return text
	}
	return append(text, flow.Open(fmt.Sprintf("在 %s 中观看", d.watchAt.Server), d.watchAt.Link))
}
