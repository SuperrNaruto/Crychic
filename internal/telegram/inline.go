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
// Telegram sends a query per keystroke.
type inlineSearches struct {
	mu      sync.Mutex
	running map[int64]context.CancelFunc
}

// begin starts user's search, cancelling the one before it.
func (s *inlineSearches) begin(ctx context.Context, user int64) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	if prev := s.running[user]; prev != nil {
		prev()
	}
	s.running[user] = cancel
	s.mu.Unlock()
	return ctx, cancel
}

// onInline answers an inline query with media from a search, each shared as
// a short card whose button opens it in a private chat with the bot. Users
// off the whitelist get no results.
func (a *adapter) onInline(ctx context.Context, b *bot.Bot, q *models.InlineQuery) {
	results := []models.InlineQueryResult{}
	if a.allowed[q.From.ID] {
		ctx, cancel := a.inline.begin(ctx, q.From.ID)
		defer cancel()
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
