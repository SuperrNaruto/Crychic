// Package telegram renders flow conversations as Telegram messages with
// inline keyboards: a command sends a new message, each button press edits it.
package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/SuperrNauto/Crychic/internal/flow"
)

const (
	// pollTimeout is the getUpdates long-poll window.
	pollTimeout = time.Minute

	cmdRequest = "request"
	cmdStart   = "start"
	cmdHelp    = "help"

	msgHelp = "发送 /request <片名> 搜索电影或剧集，并在 MoviePilot 中订阅。"
)

// Flow is the conversation engine the bot drives.
type Flow interface {
	Start(ctx context.Context, actor flow.Actor, term string) flow.Reply
	Choose(ctx context.Context, actor flow.Actor, data string) flow.Reply
}

// Config configures the Telegram bot.
type Config struct {
	Token        string
	APIURL       string
	AllowedUsers []int64
	HTTPClient   *http.Client
	Log          *slog.Logger
}

type adapter struct {
	flow    Flow
	allowed map[int64]bool
	log     *slog.Logger
}

// Run long-polls Telegram until ctx is cancelled.
func Run(ctx context.Context, cfg Config, f Flow) error {
	allowed := make(map[int64]bool, len(cfg.AllowedUsers))
	for _, id := range cfg.AllowedUsers {
		allowed[id] = true
	}
	a := &adapter{flow: f, allowed: allowed, log: cfg.Log}
	b, err := bot.New(cfg.Token,
		bot.WithServerURL(cfg.APIURL),
		bot.WithHTTPClient(pollTimeout, cfg.HTTPClient),
		bot.WithDefaultHandler(a.handle),
		bot.WithErrorsHandler(func(err error) { cfg.Log.Error("telegram", "err", err) }),
	)
	if err != nil {
		return fmt.Errorf("telegram: %w", err)
	}
	cfg.Log.Info("telegram bot started")
	b.Start(ctx)
	return nil
}

func (a *adapter) handle(ctx context.Context, b *bot.Bot, upd *models.Update) {
	switch {
	case upd.Message != nil:
		a.onMessage(ctx, b, upd.Message)
	case upd.CallbackQuery != nil:
		a.onCallback(ctx, b, upd.CallbackQuery)
	}
}

func (a *adapter) onMessage(ctx context.Context, b *bot.Bot, msg *models.Message) {
	cmd, arg, ok := parseCommand(msg.Text)
	if !ok || msg.From == nil {
		return
	}
	var reply flow.Reply
	switch {
	case cmd != cmdRequest && cmd != cmdStart && cmd != cmdHelp:
		return
	case !a.allowed[msg.From.ID]:
		reply = flow.Reply{Text: fmt.Sprintf("你没有使用权限。你的 Telegram ID：%d", msg.From.ID)}
	case cmd == cmdRequest:
		reply = a.flow.Start(ctx, flow.Actor{UserID: msg.From.ID}, arg)
	default:
		reply = flow.Reply{Text: msgHelp}
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      msg.Chat.ID,
		Text:        reply.Text,
		ReplyMarkup: keyboard(reply.Buttons),
	})
	a.logFailure("sendMessage", err)
}

// onCallback answers the query last, so the client's spinner covers the
// backend round trip and the answer can carry a notice.
func (a *adapter) onCallback(ctx context.Context, b *bot.Bot, cq *models.CallbackQuery) {
	reply := flow.Reply{Notice: "你没有使用权限。"}
	if a.allowed[cq.From.ID] {
		reply = a.flow.Choose(ctx, flow.Actor{UserID: cq.From.ID}, cq.Data)
	}
	if reply.Notice == "" && cq.Message.Message != nil {
		_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      cq.Message.Message.Chat.ID,
			MessageID:   cq.Message.Message.ID,
			Text:        reply.Text,
			ReplyMarkup: keyboard(reply.Buttons),
		})
		a.logFailure("editMessageText", err)
	}
	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cq.ID,
		Text:            reply.Notice,
	})
	a.logFailure("answerCallbackQuery", err)
}

func (a *adapter) logFailure(method string, err error) {
	if err != nil {
		a.log.Error("telegram call failed", "method", method, "err", err)
	}
}

// keyboard returns nil for no buttons, which removes the keyboard on edit.
func keyboard(rows [][]flow.Button) models.ReplyMarkup {
	if len(rows) == 0 {
		return nil
	}
	kb := make([][]models.InlineKeyboardButton, len(rows))
	for i, row := range rows {
		kb[i] = make([]models.InlineKeyboardButton, len(row))
		for j, btn := range row {
			kb[i][j] = models.InlineKeyboardButton{Text: btn.Label, CallbackData: btn.Data}
		}
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: kb}
}

// parseCommand splits "/request@SomeBot dune part two" into its command and
// argument; group chats address commands to a bot with the @ suffix.
func parseCommand(text string) (cmd, arg string, ok bool) {
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	head, rest, _ := strings.Cut(text[1:], " ")
	cmd, _, _ = strings.Cut(head, "@")
	return strings.ToLower(cmd), strings.TrimSpace(rest), true
}
