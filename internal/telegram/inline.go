package telegram

import (
	"context"
	"html"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const (
	// inlineCache is how long Telegram may reuse an answer, in seconds;
	// answers are personal, so this only spares repeated keystrokes.
	inlineCache  = 10
	msgOpenInBot = "在机器人里打开"
	msgPageLink  = "查看详情页"
)

// inlineSearches cancels a user's previous inline search when they type on:
// Telegram sends a query per keystroke. The library runs handlers at once,
// so the newest query is the one with the highest update id, not the one
// whose handler got here last.
type inlineSearches struct {
	mu      sync.Mutex
	running map[int64]inlineSearch
}

// inlineSearch is a user's inline search under way.
type inlineSearch struct {
	update int64
	cancel context.CancelFunc
}

// begin starts user's search from update, cancelling an older one; ok is
// false when a newer one is under way, which this one must leave alone.
// Only searches under way are compared: after a week without updates
// Telegram picks the next update id at random, possibly lower than any
// before, so a finished search's id must not outrank it. done cancels the
// search and forgets it.
func (s *inlineSearches) begin(ctx context.Context, user, update int64) (_ context.Context, done func(), ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, found := s.running[user]
	if found && prev.update > update {
		return ctx, func() {}, false
	}
	if found {
		prev.cancel()
	}
	ctx, cancel := context.WithCancel(ctx)
	s.running[user] = inlineSearch{update: update, cancel: cancel}
	return ctx, func() { s.end(user, update, cancel) }, true
}

// end cancels user's search from update and forgets it, unless a newer one
// took its place.
func (s *inlineSearches) end(user, update int64, cancel context.CancelFunc) {
	cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[user].update == update {
		delete(s.running, user)
	}
}

// onInline answers an inline query with media from a search, each shared as
// a short card whose button opens it in a private chat with the bot. Users
// off the whitelist get no results; a query older than one already begun
// gets no answer, like one cancelled.
func (a *adapter) onInline(ctx context.Context, b *bot.Bot, upd *models.Update) {
	q := upd.InlineQuery
	results := []models.InlineQueryResult{}
	if a.allowed[q.From.ID] {
		ctx, done, newest := a.inline.begin(ctx, q.From.ID, upd.ID)
		defer done()
		if !newest {
			return
		}
		found, err := a.flow.Find(ctx, q.Query)
		if ctx.Err() != nil {
			return
		}
		a.logFailure("inline search", err)
		for _, r := range found {
			results = append(results, a.article(r))
		}
	}
	_, err := b.AnswerInlineQuery(ctx, &bot.AnswerInlineQueryParams{
		InlineQueryID: q.ID, Results: results, CacheTime: inlineCache, IsPersonal: true,
	})
	a.logFailure("answer inline query", err)
}

// article is one inline result. Messages sent from inline results cannot be
// rich messages in this Bot API library, so the card is classic HTML.
func (a *adapter) article(r flow.InlineResult) *models.InlineQueryResultArticle {
	text := "<b>" + html.EscapeString(r.Title) + "</b>"
	if r.Description != "" {
		text += "\n" + html.EscapeString(r.Description)
	}
	if r.Link != "" {
		text += "\n" + `<a href="` + html.EscapeString(r.Link) + `">` + msgPageLink + "</a>"
	}
	art := &models.InlineQueryResultArticle{
		ID: r.Ref, Title: r.Title, Description: r.Description, ThumbnailURL: r.Thumb,
		InputMessageContent: &models.InputTextMessageContent{MessageText: text, ParseMode: models.ParseModeHTML},
	}
	if a.username != "" {
		open := models.InlineKeyboardButton{Text: msgOpenInBot, URL: "https://t.me/" + a.username + "?start=" + r.Ref}
		art.ReplyMarkup = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{open}}}
	}
	return art
}
