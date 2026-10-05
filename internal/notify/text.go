package notify

import (
	"fmt"
	"strings"

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
		flow.Plain("📥 你请求的"), flow.Strong(name), flow.Plain(" "+episodeRanges(d.episodes)+" 已入库。"),
	))
	if d.complete {
		text = append(text, flow.Line(flow.Plain("本季请求的剧集已全部入库。")))
	}
	return text
}

// episodeRanges renders sorted episode numbers compactly: E01–E03、E05.
func episodeRanges(eps []int) string {
	var parts []string
	for i := 0; i < len(eps); {
		j := i
		for j+1 < len(eps) && eps[j+1] == eps[j]+1 {
			j++
		}
		part := fmt.Sprintf("E%02d", eps[i])
		if j > i {
			part += fmt.Sprintf("–E%02d", eps[j])
		}
		parts = append(parts, part)
		i = j + 1
	}
	return strings.Join(parts, "、")
}
