// Package telegram renders flow conversations as Telegram messages with
// inline keyboards: a command sends a new message, each button press edits it.
package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
)

// help explains the single command.
var help = flow.Lines(flow.Line(
	flow.Plain("发送 "), flow.Mono("/request <片名>"), flow.Plain(" 搜索电影或剧集，并在 MoviePilot 中订阅。"),
))

// Flow is the conversation engine the bot drives.
type Flow interface {
	Start(ctx context.Context, actor flow.Actor, term string) flow.Reply
	Choose(ctx context.Context, actor flow.Actor, data string) flow.Reply
	Answer(ctx context.Context, actor flow.Actor, input, text string) flow.Reply
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

	mu      sync.Mutex
	pending map[inputKey]pendingInput
}

// inputKey identifies whose typed answer a conversation message waits for.
type inputKey struct{ chat, user int64 }

// pendingInput is a conversation message waiting for a typed answer.
type pendingInput struct {
	message int
	input   string
}

// Run long-polls Telegram until ctx is cancelled.
func Run(ctx context.Context, cfg Config, f Flow) error {
	allowed := make(map[int64]bool, len(cfg.AllowedUsers))
	for _, id := range cfg.AllowedUsers {
		allowed[id] = true
	}
	a := &adapter{flow: f, allowed: allowed, log: cfg.Log, pending: map[inputKey]pendingInput{}}
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
	if msg.From == nil {
		return
	}
	cmd, arg, ok := parseCommand(msg.Text)
	if !ok {
		a.onText(ctx, b, msg)
		return
	}
	var reply flow.Reply
	switch {
	case cmd != cmdRequest && cmd != cmdStart && cmd != cmdHelp:
		return
	case !a.allowed[msg.From.ID]:
		reply = flow.Reply{Text: flow.Lines(flow.Line(
			flow.Plain("🚫 你没有使用权限。你的 Telegram ID："),
			flow.Mono(strconv.FormatInt(msg.From.ID, 10)),
		))}
	case cmd == cmdRequest:
		reply = a.flow.Start(ctx, flow.Actor{UserID: msg.From.ID}, arg)
	default:
		reply = flow.Reply{Text: help}
	}
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:             msg.Chat.ID,
		Text:               renderHTML(reply.Text),
		ParseMode:          models.ParseModeHTML,
		LinkPreviewOptions: preview(reply.Image),
		ReplyMarkup:        keyboard(reply.Buttons),
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
		a.edit(ctx, b, editTarget{chat: cq.Message.Message.Chat.ID, user: cq.From.ID, message: cq.Message.Message.ID}, reply)
	}
	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cq.ID,
		Text:            reply.Notice,
	})
	a.logFailure("answerCallbackQuery", err)
}

// onText routes a plain message to the conversation waiting for its
// author's answer. In groups only a reply to that message counts, so
// ordinary chatter is never taken as an answer.
func (a *adapter) onText(ctx context.Context, b *bot.Bot, msg *models.Message) {
	key := inputKey{chat: msg.Chat.ID, user: msg.From.ID}
	a.mu.Lock()
	p, ok := a.pending[key]
	a.mu.Unlock()
	if !ok || !a.allowed[msg.From.ID] {
		return
	}
	isReply := msg.ReplyToMessage != nil && msg.ReplyToMessage.ID == p.message
	if msg.Chat.Type != models.ChatTypePrivate && !isReply {
		return
	}
	reply := a.flow.Answer(ctx, flow.Actor{UserID: msg.From.ID}, p.input, msg.Text)
	if reply.Notice != "" {
		// Pending inputs are keyed by user and hold the bot's own token, so
		// the flow never refuses one; keep the card rather than clobber it.
		a.log.Warn("typed answer refused", "notice", reply.Notice)
		return
	}
	a.edit(ctx, b, editTarget{chat: key.chat, user: key.user, message: p.message}, reply)
}

// editTarget is a conversation message and the user driving it.
type editTarget struct {
	chat, user int64
	message    int
}

// edit renders reply into the conversation message and records whether it
// now waits for a typed answer.
func (a *adapter) edit(ctx context.Context, b *bot.Bot, t editTarget, reply flow.Reply) {
	key := inputKey{chat: t.chat, user: t.user}
	a.mu.Lock()
	if reply.Input != "" {
		a.pending[key] = pendingInput{message: t.message, input: reply.Input}
	} else if a.pending[key].message == t.message {
		delete(a.pending, key)
	}
	a.mu.Unlock()
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:             t.chat,
		MessageID:          t.message,
		Text:               renderHTML(reply.Text),
		ParseMode:          models.ParseModeHTML,
		LinkPreviewOptions: preview(reply.Image),
		ReplyMarkup:        keyboard(reply.Buttons),
	})
	a.logFailure("editMessageText", err)
}

func (a *adapter) logFailure(method string, err error) {
	if err != nil {
		a.log.Error("telegram call failed", "method", method, "err", err)
	}
}

// preview shows the poster as a large link preview above the text; text
// messages cannot carry photos, and editing can't turn them into photo
// messages. Without an image any earlier preview is switched off.
func preview(image string) *models.LinkPreviewOptions {
	on := true
	if image == "" {
		return &models.LinkPreviewOptions{IsDisabled: &on}
	}
	return &models.LinkPreviewOptions{URL: &image, PreferLargeMedia: &on, ShowAboveText: &on}
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
