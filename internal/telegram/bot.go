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
	cmdTasks   = "tasks"
	cmdStart   = "start"
	cmdHelp    = "help"
)

// help explains the commands.
var help = flow.Lines(
	flow.Line(flow.Plain("发送 "), flow.Mono("/request <片名>"), flow.Plain(" 搜索电影或剧集，并在 MoviePilot 中订阅。")),
	flow.Line(flow.Plain("发送 "), flow.Mono("/tasks"), flow.Plain(" 查看下载中和整理中的任务，选一个实时查看进度。")),
)

// Flow is the conversation engine the bot drives.
type Flow interface {
	Start(ctx context.Context, actor flow.Actor, term string) flow.Reply
	Choose(ctx context.Context, actor flow.Actor, data string) flow.Reply
	Answer(ctx context.Context, actor flow.Actor, input, text string) flow.Reply
	Tasks(ctx context.Context, actor flow.Actor) flow.Reply
}

// Config configures the Telegram bot.
type Config struct {
	Token        string
	APIURL       string
	AllowedUsers []int64
	FollowEvery  time.Duration // how often a live reply (flow.Reply.Follow) refreshes
	HTTPClient   *http.Client
	Log          *slog.Logger
}

type adapter struct {
	flow        Flow
	allowed     map[int64]bool
	log         *slog.Logger
	followEvery time.Duration
	runCtx      context.Context

	mu        sync.Mutex
	pending   map[inputKey]pendingInput
	followers map[messageKey]*follower
}

// inputKey identifies whose typed answer a conversation message waits for.
type inputKey struct{ chat, user int64 }

// pendingInput is a conversation message waiting for a typed answer.
type pendingInput struct {
	message int
	input   string
}

// Bot is the Telegram front end: it drives a Flow and delivers notices.
type Bot struct {
	adapter *adapter
	api     *bot.Bot
	log     *slog.Logger
}

// New connects to Telegram (validating the token) without receiving
// updates yet, so notices can be wired before the bot goes live.
func New(cfg Config) (*Bot, error) {
	allowed := make(map[int64]bool, len(cfg.AllowedUsers))
	for _, id := range cfg.AllowedUsers {
		allowed[id] = true
	}
	a := &adapter{
		allowed: allowed, log: cfg.Log, followEvery: cfg.FollowEvery,
		pending: map[inputKey]pendingInput{}, followers: map[messageKey]*follower{},
	}
	api, err := bot.New(cfg.Token,
		bot.WithServerURL(cfg.APIURL),
		bot.WithHTTPClient(pollTimeout, cfg.HTTPClient),
		bot.WithDefaultHandler(a.handle),
		bot.WithErrorsHandler(func(err error) { cfg.Log.Error("telegram", "err", err) }),
	)
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}
	return &Bot{adapter: a, api: api, log: cfg.Log}, nil
}

// Run long-polls Telegram, handing conversations to f, until ctx is
// cancelled. Handlers only run once polling starts, after f is set.
func (b *Bot) Run(ctx context.Context, f Flow) {
	b.adapter.flow = f
	b.adapter.runCtx = ctx
	b.log.Info("telegram bot started")
	b.api.Start(ctx)
}

// Notify sends a notice to where the requester asked; in a group it
// mentions them so the notice reaches the right person.
func (b *Bot) Notify(ctx context.Context, to flow.Actor, text flow.Text, image string) error {
	chat, err := strconv.ParseInt(to.Address, 10, 64)
	if err != nil {
		return fmt.Errorf("telegram address %q: %w", to.Address, err)
	}
	if chat != to.UserID && len(text) > 0 {
		mention := flow.Linked(flow.Plain(to.Name), "tg://user?id="+strconv.FormatInt(to.UserID, 10))
		first := flow.Line(append([]flow.Span{mention, flow.Plain(" ")}, text[0].Spans...)...)
		text = append(flow.Lines(first), text[1:]...)
	}
	_, err = b.api.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:             chat,
		Text:               renderHTML(text),
		ParseMode:          models.ParseModeHTML,
		LinkPreviewOptions: preview(image),
	})
	return err
}

// actorOf identifies user acting in chat.
func actorOf(u models.User, chat int64) flow.Actor {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		name = u.Username
	}
	return flow.Actor{UserID: u.ID, Name: name, Address: strconv.FormatInt(chat, 10)}
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
	case cmd != cmdRequest && cmd != cmdTasks && cmd != cmdStart && cmd != cmdHelp:
		return
	case !a.allowed[msg.From.ID]:
		reply = flow.Reply{Text: flow.Lines(flow.Line(
			flow.Plain("🚫 你没有使用权限。你的 Telegram ID："),
			flow.Mono(strconv.FormatInt(msg.From.ID, 10)),
		))}
	case cmd == cmdRequest:
		reply = a.flow.Start(ctx, actorOf(*msg.From, msg.Chat.ID), arg)
	case cmd == cmdTasks:
		reply = a.flow.Tasks(ctx, actorOf(*msg.From, msg.Chat.ID))
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
	actor := actorOf(cq.From, callbackChat(cq))
	if a.allowed[cq.From.ID] {
		reply = a.flow.Choose(ctx, actor, cq.Data)
	}
	if reply.Notice == "" && cq.Message.Message != nil {
		t := editTarget{chat: cq.Message.Message.Chat.ID, user: cq.From.ID, message: cq.Message.Message.ID}
		a.unfollow(t.key())
		a.edit(ctx, b, t, reply)
		if reply.Follow != "" {
			a.follow(b, follower{target: t, actor: actor, data: reply.Follow, shown: renderHTML(reply.Text)})
		}
	}
	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cq.ID,
		Text:            reply.Notice,
	})
	a.logFailure("answerCallbackQuery", err)
}

// callbackChat is the chat a button was pressed in.
func callbackChat(cq *models.CallbackQuery) int64 {
	if cq.Message.Message != nil {
		return cq.Message.Message.Chat.ID
	}
	return cq.From.ID
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
	reply := a.flow.Answer(ctx, actorOf(*msg.From, msg.Chat.ID), p.input, msg.Text)
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
