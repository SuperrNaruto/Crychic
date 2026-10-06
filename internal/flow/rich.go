package flow

import "slices"

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
	// Tabular is a table: Head names the columns, each of Rows is a row of
	// cells.
	Tabular
	// Tick is a checklist line, ticked when On; consecutive ticks form one
	// list.
	Tick
	// Action opens Spans' link from a button inside the message.
	Action
)

// Block is one structural piece of a message.
type Block struct {
	Kind    BlockKind
	Spans   []Span
	Number  int      // Item only
	Facts   string   // Item only: secondary facts, e.g. "Dune · 2021 · 电影"
	Tag     string   // Item only: a fact singled out after the others, e.g. 你请求的
	Summary string   // Folded only: the label shown while closed
	Head    []string // Tabular only
	Rows    [][]Span // Tabular only: a span per cell
	On      bool     // Tick only
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

// Table lays rows of cells out under the column names in head, leaving
// out every column that is empty in all rows.
func Table(head []string, rows ...[]Span) Block {
	t := Block{Kind: Tabular}
	for col, name := range head {
		if !slices.ContainsFunc(rows, func(row []Span) bool { return row[col].Text != "" }) {
			continue
		}
		t.Head = append(t.Head, name)
		for i := range rows {
			if len(t.Rows) <= i {
				t.Rows = append(t.Rows, nil)
			}
			t.Rows[i] = append(t.Rows[i], rows[i][col])
		}
	}
	return t
}

// Ticked is a checklist line, ticked when on.
func Ticked(on bool, spans ...Span) Block { return Block{Kind: Tick, Spans: spans, On: on} }

// Open is a button inside the message that opens url.
func Open(label, url string) Block {
	return Block{Kind: Action, Spans: []Span{Linked(Plain(label), url)}}
}

// Lines builds a Text.
func Lines(blocks ...Block) Text { return blocks }

// Sentence is a one-line Text of plain words.
func Sentence(s string) Text { return Text{Line(Plain(s))} }
