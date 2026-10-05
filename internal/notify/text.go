package notify

import (
	"fmt"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

// arrivalText is the notice for one delivery, e.g.
// "📥 你请求的《碧蓝之海》第 3 季 E01–E03、E05 已入库。".
func arrivalText(d delivery) flow.Text {
	w := d.watch
	name := fmt.Sprintf("《%s》", w.Title)
	if w.Season == nil {
		return flow.Lines(flow.Line(flow.Plain("📥 你请求的"), flow.Strong(name), flow.Plain("已入库，可以观看了。")))
	}
	name += fmt.Sprintf("第 %d 季", *w.Season)
	text := flow.Lines(flow.Line(
		flow.Plain("📥 你请求的"), flow.Strong(name), flow.Plain(" "+flow.EpisodeRanges(d.episodes)+" 已入库。"),
	))
	if d.complete {
		text = append(text, flow.Line(flow.Plain("本季请求的剧集已全部入库。")))
	}
	return text
}
