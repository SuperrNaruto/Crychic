package e2e

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// maxResultID is Telegram's limit on an inline result's id, in bytes.
const maxResultID = 64

// inlineResult is the part of an InlineQueryResultArticle the fake checks
// and records.
type inlineResult struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Desc    string `json:"description"`
	Thumb   string `json:"thumbnail_url"`
	Content struct {
		Text      string `json:"message_text"`
		ParseMode string `json:"parse_mode"`
	} `json:"input_message_content"`
	Markup struct {
		Rows [][]struct {
			Text string `json:"text"`
			URL  string `json:"url"`
		} `json:"inline_keyboard"`
	} `json:"reply_markup"`
}

// answerInline checks an answerInlineQuery like Telegram does (results an
// array of articles with a 1–64 byte id, a title and message text) and
// records each result.
func (f *fakeTelegram) answerInline(w http.ResponseWriter, r *http.Request) {
	var results []inlineResult
	if err := json.Unmarshal([]byte(r.FormValue("results")), &results); err != nil || results == nil {
		reply(w, nil, errors.New("results must be an array"))
		return
	}
	lines := []string{fmt.Sprintf("<< answerInlineQuery personal=%s cache=%s results=%d",
		r.FormValue("is_personal"), r.FormValue("cache_time"), len(results))}
	for _, res := range results {
		if res.Type != "article" || res.ID == "" || len(res.ID) > maxResultID || res.Title == "" || res.Content.Text == "" {
			reply(w, nil, fmt.Errorf("invalid inline result %+v", res))
			return
		}
		lines = append(lines, fmt.Sprintf("   [%s] %s — %s | thumb=%s | %s", res.ID, res.Title, res.Desc, res.Thumb, res.Content.ParseMode))
		for _, line := range strings.Split(res.Content.Text, "\n") {
			lines = append(lines, "     "+line)
		}
		for _, row := range res.Markup.Rows {
			for _, b := range row {
				lines = append(lines, fmt.Sprintf("     [%s | %s]", b.Text, b.URL))
			}
		}
	}
	for _, line := range lines {
		f.tr.add(line)
	}
	f.finish("inline:" + r.FormValue("inline_query_id"))
	reply(w, true, nil)
}

// inline types query after the bot's username in any chat and waits for
// the results.
func (h *harness) inline(user int64, query string) {
	h.t.Helper()
	h.nextCB++
	id := "q" + strconv.Itoa(h.nextCB)
	h.tr.add(fmt.Sprintf(">> user %d types @crychic_bot %s", user, query))
	upd := map[string]any{"inline_query": map[string]any{
		"id": id, "query": query, "offset": "",
		"from": map[string]any{"id": user, "is_bot": false, "first_name": strconv.FormatInt(user, 10)},
	}}
	h.wait(h.tg.push(upd, "inline:"+id), "inline query "+query)
}
