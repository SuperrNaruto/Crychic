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
// lines are joined by breaks, numbered lists, checklists, tables, link
// buttons, entries that open to show more, collapsible quotes and footers. All content is escaped so titles and synopses can never break
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
	open string // "p", "footer", "ol" or "ul" while one is open
}

func (w *richWriter) add(b flow.Block) {
	switch b.Kind {
	case flow.Para:
		w.line("p", b.Spans)
	case flow.Fine:
		w.line("footer", b.Spans)
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
	case flow.Tick:
		w.tick(b)
	case flow.Tabular:
		if len(b.Head) > 0 {
			w.block(renderTable(b))
		}
	case flow.Action:
		w.block(renderAction(b.Spans))
	}
}

// line continues the open paragraph (p) or small print (footer); an
// empty line ends it.
func (w *richWriter) line(tag string, spans []flow.Span) {
	if len(spans) == 0 {
		w.close()
		return
	}
	if w.open == tag {
		w.WriteString("<br>")
	} else {
		w.block("<" + tag + ">")
		w.open = tag
	}
	w.WriteString(renderSpans(spans))
}

// item continues the open list; each entry keeps its own number, so a
// later page still matches its buttons. An entry with a body folds it
// behind its name and facts, which open it.
func (w *richWriter) item(b flow.Block) {
	if w.open != "ol" {
		w.block(fmt.Sprintf(`<ol start="%d">`, b.Number))
		w.open = "ol"
	}
	head := renderSpans(b.Spans) + renderFacts(b.Facts, b.Tag)
	if len(b.Body) > 0 {
		head = "<details><summary>" + head + "</summary>" + renderRich(b.Body, "") + "</details>"
	}
	fmt.Fprintf(w, `<li value="%d">%s</li>`, b.Number, head)
}

// renderFacts puts an entry's facts in italics on a line below its name,
// its tag highlighted after them.
func renderFacts(facts, tag string) string {
	if facts == "" && tag == "" {
		return ""
	}
	out := html.EscapeString(facts)
	if tag != "" && facts != "" {
		out += " · "
	}
	if tag != "" {
		out += "<mark>" + html.EscapeString(tag) + "</mark>"
	}
	return "<br><i>" + out + "</i>"
}

// tick continues the open checklist.
func (w *richWriter) tick(b flow.Block) {
	if w.open != "ul" {
		w.block("<ul>")
		w.open = "ul"
	}
	box := `<input type="checkbox">`
	if b.On {
		box = `<input type="checkbox" checked>`
	}
	w.WriteString("<li>" + box + renderSpans(b.Spans) + "</li>")
}

// renderTable is a compact bordered table, the column names on top.
func renderTable(b flow.Block) string {
	var t strings.Builder
	t.WriteString("<table bordered compact><tr>")
	for _, name := range b.Head {
		t.WriteString("<th>" + html.EscapeString(name) + "</th>")
	}
	t.WriteString("</tr>")
	for _, row := range b.Rows {
		t.WriteString("<tr>")
		for _, cell := range row {
			t.WriteString("<td>" + renderSpan(cell) + "</td>")
		}
		t.WriteString("</tr>")
	}
	return t.String() + "</table>"
}

// renderAction is a button opening the spans' link, left aligned.
func renderAction(spans []flow.Span) string {
	var row strings.Builder
	row.WriteString(`<tg-button-row align="left">`)
	for _, s := range spans {
		row.WriteString(`<tg-button type="url" style="success" url="` + html.EscapeString(s.Link) + `">` +
			html.EscapeString(s.Text) + "</tg-button>")
	}
	return row.String() + "</tg-button-row>"
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
