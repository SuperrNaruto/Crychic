package flow

import "fmt"

const (
	actionPage     = "p"
	listPageItems  = 20
	listPageRunes  = 3000 // room for headings and navigation within Telegram's limit
	listTitleRunes = 100
)

type listEntry struct {
	text    Text
	buttons []Button
	poster  string // shown in the page's gallery when set
}

type listView struct {
	heading   Block
	entries   []listEntry
	footer    []Button
	note      string
	pageItems int // zero uses the default; search pages use fewer entries
}

// listPages keeps text and its action buttons on the same bounded page,
// the buttons gridColumns a row.
func (e *Engine) listPages(sess session, view listView) Reply {
	maxItems := view.pageItems
	if maxItems <= 0 {
		maxItems = listPageItems
	}
	pages := []Reply{{Text: Lines(view.heading)}}
	buttons := [][]Button{nil}
	used, items := 0, 0
	for _, entry := range view.entries {
		size := textSize(entry.text)
		if items > 0 && (items >= maxItems || used+size > listPageRunes) {
			pages, buttons = append(pages, Reply{Text: Lines(view.heading)}), append(buttons, nil)
			used, items = 0, 0
		}
		last := len(pages) - 1
		pages[last].Text = append(pages[last].Text, entry.text...)
		buttons[last] = append(buttons[last], entry.buttons...)
		if entry.poster != "" {
			pages[last].Gallery = append(pages[last].Gallery, entry.poster)
		}
		used, items = used+size, items+1
	}
	for i := range pages {
		pages[i].Buttons = grid(buttons[i], gridColumns)
		if view.note != "" {
			pages[i].Text = append(pages[i].Text, Remark(view.note))
		}
		if len(pages) > 1 {
			pages[i].Text = append(pages[i].Text, Remark(fmt.Sprintf("第 %d/%d 页", i+1, len(pages))))
			pages[i].Buttons = append(pages[i].Buttons, listPager(sess.id, i, len(pages)))
		}
		pages[i].Buttons = append(pages[i].Buttons, view.footer)
	}
	sess.pages = pages
	e.store.put(sess)
	return pages[0]
}

func textSize(text Text) int {
	n := len(text)
	for _, block := range text {
		n += len([]rune(block.Facts)) + len([]rune(block.Tag)) + textSize(block.Body)
		for _, span := range block.Spans {
			n += len([]rune(span.Text))
		}
		for _, head := range block.Head {
			n += len([]rune(head))
		}
		for _, row := range block.Rows {
			for _, cell := range row {
				n += len([]rune(cell.Text))
			}
		}
	}
	return n
}

func listPager(id uint64, page, count int) []Button {
	var row []Button
	if page > 0 {
		row = append(row, Button{Label: "‹ 上一页", Data: data(id, actionPage, page-1)})
	}
	if page+1 < count {
		row = append(row, Button{Label: "下一页 ›", Data: data(id, actionPage, page+1)})
	}
	return row
}

func (e *Engine) page(sess session, index int) Reply {
	if index < 0 || index >= len(sess.pages) {
		return Reply{Notice: msgInvalidChoice}
	}
	return e.navigate(sess, actionPage, sess.pages[index])
}
