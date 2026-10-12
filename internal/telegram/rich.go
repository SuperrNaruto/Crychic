package telegram

import (
	"fmt"
	"html"
	"strings"

	"github.com/go-telegram/bot/models"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// richMessage renders flow text as a Telegram rich message, with image (an
// http(s) URL or a tg://photo link to uploaded media) as a block on top.
func richMessage(reply flow.Reply) *models.InputRichMessage {
	return &models.InputRichMessage{HTML: renderReply(reply)}
}

// renderReply is reply as rich HTML: its poster above the text.
func renderReply(reply flow.Reply) string {
	if reply.Image == "" {
		return renderRich(reply.Text, "")
	}
	return renderRich(reply.Text, imgTag(reply.Image))
}

func imgTag(url string) string { return `<img src="` + html.EscapeString(url) + `"/>` }

// renderRich turns flow text into rich HTML: a heading, paragraphs whose
// lines are joined by breaks, numbered lists, checklists, tables, link
// buttons, entries that open to show more, collapsible quotes and footers. All content is escaped so titles and synopses can never break
// the markup.
func renderRich(text flow.Text, media string) string {
	var w richWriter
	if media != "" {
		w.block(media)
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
	open string // "p", "footer", "blockquote", "ol" or "ul" while one is open
}

// lineTags are the blocks whose consecutive lines share one element.
var lineTags = map[flow.BlockKind]string{flow.Para: "p", flow.Fine: "footer", flow.Alert: "blockquote"}

// spanTags wrap a block's spans in one element of their own.
var spanTags = map[flow.BlockKind][2]string{
	flow.Title:   {"<h3>", "</h3>"},
	flow.Section: {"<h4>", "</h4>"},
	flow.Quoted:  {"<blockquote expandable>", "</blockquote>"},
	flow.Note:    {"<footer>", "</footer>"},
}

func (w *richWriter) add(b flow.Block) {
	if tag, ok := lineTags[b.Kind]; ok {
		w.line(tag, b.Spans)
		return
	}
	if tags, ok := spanTags[b.Kind]; ok {
		w.block(tags[0] + renderSpans(b.Spans) + tags[1])
		return
	}
	switch b.Kind {
	case flow.Folded:
		w.block("<details><summary>" + renderSpans(b.Spans) + "</summary>" + renderRich(b.Body, "") + "</details>")
	case flow.Item:
		w.item(b)
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

// line continues the open paragraph (p), small print (footer) or callout (blockquote); an
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
	head := renderSpans(b.Spans) + renderFacts(b)
	if len(b.Body) > 0 {
		head = "<details><summary>" + head + "</summary>" + renderRich(b.Body, "") + "</details>"
	}
	fmt.Fprintf(w, `<li value="%d">%s</li>`, b.Number, head)
}

// renderFacts puts an entry's facts in italics on a line below its name,
// or after it for a brief entry, its tag highlighted after them.
func renderFacts(b flow.Block) string {
	facts, tag := b.Facts, b.Tag
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
	if b.Brief {
		return " · <i>" + out + "</i>"
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
	t.WriteString("<table bordered striped compact><tr>")
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
