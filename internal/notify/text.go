package notify

import (
	"fmt"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

// arrivedHeading opens every arrival notice.
var arrivedHeading = flow.Heading(flow.Plain("📥 入库啦"))

// arrivalText is the notice for one delivery, e.g. "📥 入库啦" over
// "你想看的《碧蓝之海》第 3 季 E01–E03、E05 到家啦！".
func arrivalText(d delivery) flow.Text {
	w := d.watch
	name := fmt.Sprintf("《%s》", w.Title)
	if w.Season == nil {
		return withWatchLink(flow.Lines(arrivedHeading, flow.Line(flow.Plain("你想看的"), flow.Strong(name), flow.Plain("到家啦，快去看吧！(ﾉ>ω<)ﾉ"))), d)
	}
	name += fmt.Sprintf("第 %d 季", *w.Season)
	text := flow.Lines(arrivedHeading, flow.Line(
		flow.Plain("你想看的"), flow.Strong(name), flow.Plain(" "+flow.EpisodeRanges(d.episodes)+" 到家啦！"),
	))
	if d.complete {
		text = append(text, flow.Line(flow.Plain("这一季你要的剧集全部到齐啦 ヾ(≧▽≦*)o")))
	}
	return withWatchLink(text, d)
}

// withWatchLink adds a button to where to watch, e.g. "在 Emby 中观看".
func withWatchLink(text flow.Text, d delivery) flow.Text {
	if d.watchAt.Link == "" {
		return text
	}
	return append(text, flow.Open(fmt.Sprintf("在 %s 中观看", d.watchAt.Server), d.watchAt.Link))
}
