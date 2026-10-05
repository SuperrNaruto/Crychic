package telegram

import (
	"html"
	"strings"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

// widthPad is a last line of blank Braille cells (U+2800, which Telegram
// does not trim as whitespace). Clients size a bubble to its longest line, so
// without it a message shrinks and grows as edits change its text; messages
// with a poster are already as wide as the preview makes them. widthPadCells
// stays short enough not to wrap on a narrow phone.
const (
	widthPadCell  = "\u2800"
	widthPadCells = 28
)

var widthPad = strings.Repeat(widthPadCell, widthPadCells)

// messageHTML is a message's HTML: text, padded to a steady width unless a
// poster preview sets it.
func messageHTML(text flow.Text, image string) string {
	if image != "" {
		return renderHTML(text)
	}
	return renderHTML(text) + "\n" + widthPad
}

// renderHTML turns flow text into Telegram's HTML parse mode, escaping all
// content so titles and synopses can never break the markup.
func renderHTML(text flow.Text) string {
	lines := make([]string, len(text))
	for i, block := range text {
		var b strings.Builder
		for _, span := range block.Spans {
			b.WriteString(renderSpan(span))
		}
		lines[i] = b.String()
		if block.Quote {
			lines[i] = "<blockquote expandable>" + lines[i] + "</blockquote>"
		}
	}
	return strings.Join(lines, "\n")
}

func renderSpan(s flow.Span) string {
	out := html.EscapeString(s.Text)
	if s.Style&flow.Code != 0 {
		out = "<code>" + out + "</code>"
	}
	if s.Style&flow.Italic != 0 {
		out = "<i>" + out + "</i>"
	}
	if s.Style&flow.Bold != 0 {
		out = "<b>" + out + "</b>"
	}
	if s.Link != "" {
		out = `<a href="` + html.EscapeString(s.Link) + `">` + out + "</a>"
	}
	return out
}
