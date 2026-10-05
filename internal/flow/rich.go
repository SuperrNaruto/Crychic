package flow

// Text is formatted message content described by meaning, not markup;
// each platform adapter renders it in its own syntax and does the escaping.
type Text []Block

// Block is one line of spans, or a collapsible quote.
type Block struct {
	Quote bool
	Spans []Span
}

// Style marks how a span is emphasised.
type Style uint8

const (
	Bold Style = 1 << iota
	Italic
	Code
)

// Span is a run of text with one style, optionally linked.
type Span struct {
	Text  string
	Style Style
	Link  string
}

func Plain(s string) Span            { return Span{Text: s} }
func Strong(s string) Span           { return Span{Text: s, Style: Bold} }
func Emphasis(s string) Span         { return Span{Text: s, Style: Italic} }
func Mono(s string) Span             { return Span{Text: s, Style: Code} }
func Linked(s Span, url string) Span { return Span{Text: s.Text, Style: s.Style, Link: url} }

// Line is a block of spans; Line() is an empty line.
func Line(spans ...Span) Block { return Block{Spans: spans} }

// Quote is a collapsible quoted block, used for long synopses.
func Quote(s string) Block { return Block{Quote: true, Spans: []Span{Plain(s)}} }

// Lines builds a Text.
func Lines(blocks ...Block) Text { return blocks }

// Sentence is a one-line Text of plain words.
func Sentence(s string) Text { return Text{Line(Plain(s))} }
