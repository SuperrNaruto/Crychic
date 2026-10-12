package flow

import "fmt"

const (
	actionPage       = "p"
	listPageItems    = 20
	compactPageItems = 8    // one-line lists: latest items and subscriptions
	listPageRunes    = 3000 // room for headings and navigation within Telegram's limit
	listTitleRunes   = 100
	subscriptionList = "subscriptions"
	upcomingList     = "upcoming"
)

type listEntry struct {
	text    Text
	buttons []Button
	poster  string // prefetched with the page when set
	section string // a Group above the entry when it opens a page or a new section
}

type listView struct {
	heading   Block
	image     string // one poster shared by the pages of a detail view
	entries   []listEntry
	menu      [][]Button // rows above the footer on every page
	footer    []Button
	note      string
	notes     Text   // blocks after the note, on every page
	pageItems int    // zero uses the default; search pages use fewer entries
	source    string // list provenance for navigation across subscription details
}

// listPages keeps text and its action buttons on the same bounded page,
// the buttons gridColumns a row, and shows the first page.
func (e *Engine) listPages(sess session, view listView) Reply {
	return e.listPageWith(sess, view, "")
}

// listPageWith pages view like listPages and shows the page holding the
// button with data holds, or the first page.
func (e *Engine) listPageWith(sess session, view listView, holds string) Reply {
	sess.pages = paged(sess.id, view)
	sess.listing, sess.pageIndex = view.source, 0
	for index, page := range sess.pages {
		if holds != "" && hasData(page.Buttons, holds) {
			sess.pageIndex = index
			break
		}
	}
	e.store.put(sess)
	return sess.pages[sess.pageIndex]
}

func hasData(rows [][]Button, d string) bool {
	for _, row := range rows {
		for _, b := range row {
			if b.Data == d {
				return true
			}
		}
	}
	return false
}

// paged lays view out on bounded pages.
func paged(id uint64, view listView) []Reply {
	maxItems := view.pageItems
	if maxItems <= 0 {
		maxItems = listPageItems
	}
	pages := []Reply{{Text: Lines(view.heading), Image: view.image}}
	buttons := [][]Button{nil}
	used, items, section := 0, 0, ""
	for _, entry := range view.entries {
		size := textSize(entry.text)
		if items > 0 && (items >= maxItems || used+size > listPageRunes) {
			pages, buttons = append(pages, Reply{Text: Lines(view.heading), Image: view.image}), append(buttons, nil)
			used, items = 0, 0
		}
		last := len(pages) - 1
		placeEntry(&pages[last], entry, items == 0 || entry.section != section)
		buttons[last] = append(buttons[last], entry.buttons...)
		pages[last].Prefetch = withPoster(pages[last].Prefetch, entry.poster)
		used, items, section = used+size, items+1, entry.section
	}
	for i := range pages {
		pages[i].Buttons = grid(buttons[i], gridColumns)
		if view.note != "" {
			pages[i].Text = append(pages[i].Text, Remark(view.note))
		}
		pages[i].Text = append(pages[i].Text, view.notes...)
		if len(pages) > 1 {
			pages[i].Text = append(pages[i].Text, Remark(fmt.Sprintf("第 %d/%d 页", i+1, len(pages))))
			pages[i].Buttons = append(pages[i].Buttons, listPager(id, i, len(pages)))
		}
		pages[i].Buttons = append(append(pages[i].Buttons, view.menu...), view.footer)
	}
	return pages
}

// withPoster appends poster to urls when there is one.
func withPoster(urls []string, poster string) []string {
	if poster == "" {
		return urls
	}
	return append(urls, poster)
}

// placeEntry adds entry to page, under its section's Group when it opens
// the page or a new section.
func placeEntry(page *Reply, entry listEntry, opens bool) {
	if entry.section != "" && opens {
		page.Text = append(page.Text, Group(entry.section))
	}
	page.Text = append(page.Text, entry.text...)
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
	sess.pageIndex = index
	e.store.put(sess)
	return e.navigate(sess, actionPage, sess.pages[index])
}
