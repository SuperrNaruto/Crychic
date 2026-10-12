package notify

import (
	"regexp"
	"strings"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// nativeField is one MoviePilot template fact such as 「质量：WEB-DL 1080p」:
// a short label without spaces or punctuation, then its value.
var nativeField = regexp.MustCompile(`^([^\s：:，,。；;]{1,10})：(.*)$`)

// nativeFieldSep joins facts on one line, as in the 入库 template.
const nativeFieldSep = "，"

var nativeFieldHead = []string{"项目", "内容"}

// nativeIcons pick a heading emoji from the words of MoviePilot's title,
// the first match winning; other notices get nativeIcon.
var nativeIcons = []struct{ word, icon string }{
	{"失败", "⚠️"}, {"错误", "⚠️"}, {"入库", "📥"}, {"下载", "⬇️"}, {"订阅", "📚"},
}

const nativeIcon = "🔔"

func nativeHeading(title string) flow.Block {
	icon := nativeIcon
	for _, i := range nativeIcons {
		if strings.Contains(title, i.word) {
			icon = i.icon
			break
		}
	}
	return flow.Heading(flow.Plain(icon + " " + title))
}

// nativeBody lays MoviePilot's text out: each run of fact lines becomes one
// table, every other line stays a paragraph line.
func nativeBody(text string) flow.Text {
	var body flow.Text
	var rows [][]flow.Span
	flush := func() {
		if len(rows) > 0 {
			body = append(body, flow.Table(nativeFieldHead, rows...))
			rows = nil
		}
	}
	for _, line := range strings.Split(text, "\n") {
		fields := nativeFields(line)
		if fields == nil {
			flush()
			body = append(body, flow.Line(flow.Plain(line)))
			continue
		}
		rows = append(rows, fields...)
	}
	flush()
	return body
}

// nativeFields splits a line opening with a fact into table rows. A segment
// without a label stands alone when a later fact follows it (「共3个文件」)
// and otherwise continues the previous value, keeping a synopsis's commas.
func nativeFields(line string) [][]flow.Span {
	segments := strings.Split(line, nativeFieldSep)
	if !nativeField.MatchString(segments[0]) {
		return nil
	}
	last := lastNativeField(segments)
	var rows [][]flow.Span
	for i, segment := range segments {
		if m := nativeField.FindStringSubmatch(segment); m != nil {
			rows = append(rows, []flow.Span{flow.Plain(m[1]), flow.Plain(m[2])})
		} else if i < last {
			rows = append(rows, []flow.Span{flow.Plain(""), flow.Plain(segment)})
		} else {
			value := &rows[len(rows)-1][1]
			*value = flow.Plain(value.Text + nativeFieldSep + segment)
		}
	}
	return rows
}

func lastNativeField(segments []string) int {
	for i := len(segments) - 1; i >= 0; i-- {
		if nativeField.MatchString(segments[i]) {
			return i
		}
	}
	return 0
}
