package telegram

import (
	"fmt"
	"html"
	"strings"

	"github.com/go-telegram/bot/models"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

// richMessage renders flow text as a Telegram rich message, with image (an
// http(s) URL or a tg://photo link to uploaded media) as a block on top.
func richMessage(text flow.Text, image string) *models.InputRichMessage {
	return &models.InputRichMessage{HTML: renderRich(text, image)}
}

// renderRich turns flow text into rich HTML: a heading, paragraphs whose
// lines are joined by breaks, numbered lists, collapsible quotes, closed
// details and footers. All content is escaped so titles and synopses can never break
// the markup.
func renderRich(text flow.Text, image string) string {
	var w richWriter
	if image != "" {
		w.block(`<img src="` + html.EscapeString(image) + `"/>`)
	}
	for _, b := range text {
		w.add(b)
	}
	w.close()
	return w.String()
}

// richWriter writes blocks, keeping one paragraph or list open while
// consecutive lines or items continue it.
type richWriter struct {
	strings.Builder
	open string // "p" or "ol" while one is open
}

func (w *richWriter) add(b flow.Block) {
	switch b.Kind {
	case flow.Para:
		w.line(b.Spans)
	case flow.Item:
		w.item(b)
	case flow.Title:
		w.block("<h3>" + renderSpans(b.Spans) + "</h3>")
	case flow.Section:
		w.block("<h4>" + renderSpans(b.Spans) + "</h4>")
	case flow.Quoted:
		w.block("<blockquote expandable>" + renderSpans(b.Spans) + "</blockquote>")
	case flow.Note:
		w.block("<footer>" + renderSpans(b.Spans) + "</footer>")
	case flow.Rule:
		w.block("<hr/>")
	case flow.Folded:
		w.block("<details><summary>" + html.EscapeString(b.Summary) + "</summary><p>" +
			renderSpans(b.Spans) + "</p></details>")
	}
}

// line continues the open paragraph; an empty line ends it.
func (w *richWriter) line(spans []flow.Span) {
	if len(spans) == 0 {
		w.close()
		return
	}
	if w.open == "p" {
		w.WriteString("<br>")
	} else {
		w.block("<p>")
		w.open = "p"
	}
	w.WriteString(renderSpans(spans))
}

// item continues the open list; each entry keeps its own number, so a
// later page still matches its buttons.
func (w *richWriter) item(b flow.Block) {
	if w.open != "ol" {
		w.block(fmt.Sprintf(`<ol start="%d">`, b.Number))
		w.open = "ol"
	}
	fmt.Fprintf(w, `<li value="%d">%s`, b.Number, renderSpans(b.Spans))
	if b.Facts != "" {
		w.WriteString("<br><i>" + html.EscapeString(b.Facts) + "</i>")
	}
	w.WriteString("</li>")
}

// block starts a top-level block, closing any open paragraph or list.
func (w *richWriter) block(markup string) {
	w.close()
	w.WriteString(markup)
}

func (w *richWriter) close() {
	if w.open != "" {
		w.WriteString("</" + w.open + ">")
		w.open = ""
	}
}

func renderSpans(spans []flow.Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(renderSpan(s))
	}
	return b.String()
}

// renderSpan escapes a span; rich HTML collapses newlines like a browser,
// so line breaks inside text (multi-line synopses) become explicit.
func renderSpan(s flow.Span) string {
	out := strings.ReplaceAll(html.EscapeString(s.Text), "\n", "<br>")
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
