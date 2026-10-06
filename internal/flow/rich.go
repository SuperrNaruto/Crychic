package flow

// Text is formatted message content described by meaning, not markup;
// each platform adapter renders it in its own syntax and does the escaping.
type Text []Block

// BlockKind is what a block is for; adapters decide how each kind looks.
type BlockKind uint8

const (
	// Para is a line of a paragraph; consecutive lines form one paragraph
	// and an empty Line() ends it.
	Para BlockKind = iota
	// Title names the whole message.
	Title
	// Section heads a group of entries within a message.
	Section
	// Item is a numbered entry: its name in Spans, its facts below.
	Item
	// Quoted is a collapsible quote, used for long synopses.
	Quoted
	// Note is a secondary remark after the content (page, refresh state).
	Note
	// Rule separates a media card from the question below it.
	Rule
	// Folded is hidden behind its always shown Summary until opened, used
	// for the synopses of list entries.
	Folded
)

// Block is one structural piece of a message.
type Block struct {
	Kind    BlockKind
	Spans   []Span
	Number  int    // Item only
	Facts   string // Item only: secondary facts, e.g. "Dune · 2021 · 电影"
	Summary string // Folded only: the label shown while closed
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

// Line is a paragraph line of spans; Line() ends the paragraph.
func Line(spans ...Span) Block { return Block{Spans: spans} }

// Heading names the message.
func Heading(spans ...Span) Block { return Block{Kind: Title, Spans: spans} }

// Group heads a group of entries.
func Group(s string) Block { return Block{Kind: Section, Spans: []Span{Plain(s)}} }

// Remark is a secondary note after the content.
func Remark(s string) Block { return Block{Kind: Note, Spans: []Span{Plain(s)}} }

// Divider separates a card from what follows.
func Divider() Block { return Block{Kind: Rule} }

// Quote is a collapsible quoted block, used for long synopses.
func Quote(s string) Block { return Block{Kind: Quoted, Spans: []Span{Plain(s)}} }

// Fold hides body behind summary until the reader opens it.
func Fold(summary, body string) Block {
	return Block{Kind: Folded, Spans: []Span{Plain(body)}, Summary: summary}
}

// Lines builds a Text.
func Lines(blocks ...Block) Text { return blocks }

// Sentence is a one-line Text of plain words.
func Sentence(s string) Text { return Text{Line(Plain(s))} }
