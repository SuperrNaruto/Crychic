// Package telegram renders flow conversations as Telegram messages with
// inline keyboards: a command sends a new message, each button press edits it.
package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const (
	// pollTimeout is the getUpdates long-poll window.
	pollTimeout = time.Minute

	// decimal is the base Telegram ids are written in.
	decimal = 10

	cmdSearch     = "search"
	cmdTasks      = "tasks"
	cmdTrending   = "trending"
	cmdSubscribe  = "subscribe"
	cmdNewlyAdded = "newly_added"
	cmdUpcoming   = "upcoming"
	cmdStart      = "start"
	cmdHelp       = "help"
)

// help explains the commands.
var help = flow.Lines(
	flow.Heading(flow.Plain("我能帮你做这些～")),
	flow.Line(flow.Plain("私聊直接发片名我就去搜，片名前加「下载」我会选好后直接搜资源；在等你回复起始集数的时候，先回答我，或者用 /search 换一部～")),
	flow.Line(flow.Mono("/start"), flow.Plain(" 回到首页，所有功能都在这儿～")),
	flow.Line(flow.Plain("在任意聊天输入 "), flow.Mono("@我的用户名 片名"), flow.Plain(" 可以把片子分享出去，点卡片上的按钮回到我这儿订阅～")),
	flow.Line(flow.Mono("/search <片名>"), flow.Plain(" 帮你搜电影和剧集，顺手在 MoviePilot 里订上；也能点「搜索资源」挑种子直接下载，一次可以多选哦～")),
	flow.Line(flow.Mono("/trending"), flow.Plain(" 逛逛热门榜单和新番，看中了一键订阅～")),
	flow.Line(flow.Mono("/subscribe"), flow.Plain(" 按电视剧、电影看订阅，还能暂停或取消你请求的、从订阅历史重新订阅～")),
	flow.Line(flow.Mono("/upcoming"), flow.Plain(" 追剧日历：你订阅的剧接下来 7 天播哪几集～")),
	flow.Line(flow.Mono("/newly_added"), flow.Plain(" 媒体库新到的片子，点片名直接去看～")),
	flow.Line(flow.Mono("/tasks"), flow.Plain(" 看看下载和整理的进度，选一个我帮你实时盯着，不要的下载也能删掉～")),
)

// allowedUpdates are the update types the bot handles. Telegram keeps the
// last list a poll sent, so leaving it out would keep whatever an earlier
// program set on the token (the owner's only allowed messages and button
// presses, which silenced inline queries).
var allowedUpdates = bot.AllowedUpdates{
	models.AllowedUpdateMessage, models.AllowedUpdateCallbackQuery, models.AllowedUpdateInlineQuery,
}

// commands is the menu Telegram shows when a user types "/".
var commands = []models.BotCommand{
	{Command: cmdStart, Description: "首页：所有功能入口"},
	{Command: cmdSearch, Description: "搜索电影或剧集，订阅或下载"},
	{Command: cmdTrending, Description: "发现热门和新番"},
	{Command: cmdSubscribe, Description: "查看、暂停和取消订阅"},
	{Command: cmdUpcoming, Description: "追剧日历：订阅的剧这周播哪集"},
	{Command: cmdNewlyAdded, Description: "最新入库"},
	{Command: cmdTasks, Description: "查看进度，删除下载任务"},
	{Command: cmdHelp, Description: "使用说明"},
}

// Config configures the Telegram bot.
type Config struct {
	Token        string
	APIURL       string
	AllowedUsers []int64
	FollowEvery  time.Duration // how often a live reply (flow.Reply.Follow) refreshes
	HTTPClient   *http.Client
	ImageClient  *http.Client // downloads posters Telegram cannot fetch itself
	Log          *slog.Logger
}

type adapter struct {
	flow        flow.Conversation
	allowed     map[int64]bool
	log         *slog.Logger
	followEvery time.Duration
	posters     *posters
	runCtx      context.Context
	api         *bot.Bot // set once connected, before any update arrives
	username    string   // the bot's username, for deep links; "" when unknown
	inline      inlineSearches

	// work counts running handlers and followers: Telegram's library
	// starts a goroutine per update and does not wait for them on stop.
	work    sync.WaitGroup
	stopped bool // set once stopping; later updates are dropped

	mu        sync.Mutex
	pending   map[inputKey]pendingInput
	followers map[messageKey]*follower
	lanes     map[messageKey]*messageLane
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
		allowed: allowed, log: cfg.Log, followEvery: cfg.FollowEvery, posters: newPosters(cfg.ImageClient, cfg.Log),
		pending: map[inputKey]pendingInput{}, followers: map[messageKey]*follower{}, lanes: map[messageKey]*messageLane{},
		inline: inlineSearches{running: map[int64]context.CancelFunc{}},
	}
	api, err := bot.New(cfg.Token,
		bot.WithServerURL(cfg.APIURL),
		bot.WithHTTPClient(pollTimeout, deadlineClient{http: cfg.HTTPClient}),
		bot.WithDefaultHandler(a.handle),
		bot.WithErrorsHandler(func(err error) { cfg.Log.Error("telegram", "err", err) }),
		bot.WithSkipGetMe(),
		bot.WithAllowedUpdates(allowedUpdates),
	)
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}
	a.api = api
	// getMe validates the token and names the bot for deep links.
	me, err := api.GetMe(context.Background())
	if err != nil {
		return nil, fmt.Errorf("telegram: getMe: %w", err)
	}
	a.username = me.Username
	return &Bot{adapter: a, api: api, log: cfg.Log}, nil
}

// Run long-polls Telegram, handing conversations to f, until ctx is
// cancelled. Handlers only run once polling starts, after f is set.
func (b *Bot) Run(ctx context.Context, f flow.Conversation) {
	b.adapter.flow = f
	b.adapter.runCtx = ctx
	b.registerCommands(ctx)
	b.log.Info("telegram bot started")
	b.api.Start(ctx)
	b.adapter.drain()
}

// drain waits for the handlers and followers still running when polling
// stopped; their context is cancelled, so they end promptly.
func (a *adapter) drain() {
	a.mu.Lock()
	a.stopped = true
	a.mu.Unlock()
	a.work.Wait()
}

// enter counts a handler as running, unless the bot is stopping.
func (a *adapter) enter() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped {
		return false
	}
	a.work.Add(1)
	return true
}

// Notify sends a notice to its destination; requester notices in groups
// include a mention. Broadcasts have no UserID and disclose no requester.
func (b *Bot) Notify(ctx context.Context, n flow.Notice) error {
	to, text := n.To, n.Text
	if to.UserID == 0 {
		_, err := b.adapter.send(ctx, to.Address, flow.Reply{Text: text, Image: n.Image})
		return err
	}
	chat, err := strconv.ParseInt(to.Address, 10, 64)
	if err != nil {
		return fmt.Errorf("telegram address %q: %w", to.Address, err)
	}
	if chat != to.UserID {
		text = withMention(text, to)
	}
	_, err = b.adapter.send(ctx, chat, flow.Reply{Text: text, Image: n.Image})
	return err
}

// withMention starts the notice's first paragraph line with a mention of
// the requester, leaving its heading on top.
func withMention(text flow.Text, to flow.Actor) flow.Text {
	i := slices.IndexFunc(text, func(b flow.Block) bool { return b.Kind == flow.Para && len(b.Spans) > 0 })
	if i < 0 {
		return text
	}
	mention := flow.Linked(flow.Plain(to.Name), "tg://user?id="+strconv.FormatInt(to.UserID, decimal))
	out := slices.Clone(text)
	out[i] = flow.Line(append([]flow.Span{mention, flow.Plain(" ")}, text[i].Spans...)...)
	return out
}

// registerCommands sets the "/" menu on every start, keeping it in step
// with the commands. A menu set for one chat outranks the default one, so
// each whitelisted user's private chat gets it too, replacing whatever an
// earlier program left there. A failure only costs the menu.
func (b *Bot) registerCommands(ctx context.Context) {
	for id := range b.adapter.allowed {
		scope := &models.BotCommandScopeChat{ChatID: id}
		if _, err := b.api.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: commands, Scope: scope}); err != nil {
			b.log.Warn("command menu not registered for user", "user", id, "err", err)
		}
	}
	if _, err := b.api.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: commands}); err != nil {
		b.log.Warn("command menu not registered", "err", err)
	}
}

// actorOf identifies user acting in chat.
func actorOf(u models.User, chat int64) flow.Actor {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		name = u.Username
	}
	return flow.Actor{UserID: u.ID, Name: name, Address: strconv.FormatInt(chat, decimal)}
}

func (a *adapter) handle(ctx context.Context, b *bot.Bot, upd *models.Update) {
	if !a.enter() {
		return
	}
	defer a.work.Done()
	switch {
	case upd.Message != nil:
		a.onMessage(ctx, b, upd.Message)
	case upd.CallbackQuery != nil:
		a.onCallback(ctx, b, upd.CallbackQuery)
	case upd.InlineQuery != nil && upd.InlineQuery.From != nil:
		a.onInline(ctx, b, upd.InlineQuery)
	}
}

func (a *adapter) onMessage(ctx context.Context, b *bot.Bot, msg *models.Message) {
	if msg.From == nil {
		return
	}
	unlock, locked := a.lockInput(ctx, msg)
	if !locked {
		return
	}
	defer unlock()
	cmd, arg, ok := parseCommand(msg.Text)
	if !ok {
		a.onText(ctx, msg)
		return
	}
	run, known := a.commands()[cmd]
	if !known {
		return
	}
	reply := refused(msg.From.ID)
	if a.allowed[msg.From.ID] {
		if msg.Chat.Type == models.ChatTypePrivate && (cmd == cmdStart || cmd == cmdSearch) {
			a.mu.Lock()
			delete(a.pending, inputKey{chat: msg.Chat.ID, user: msg.From.ID})
			a.mu.Unlock()
		}
		reply = run(ctx, actorOf(*msg.From, msg.Chat.ID), arg)
	}
	a.replyTo(ctx, msg, reply)
}

// command answers one slash command with its argument.
type command func(ctx context.Context, actor flow.Actor, arg string) flow.Reply

// commands maps every command the bot answers to its handler.
func (a *adapter) commands() map[string]command {
	noArg := func(f func(context.Context, flow.Actor) flow.Reply) command {
		return func(ctx context.Context, actor flow.Actor, _ string) flow.Reply { return f(ctx, actor) }
	}
	return map[string]command{
		cmdStart:      a.start,
		cmdSearch:     a.flow.Start,
		cmdTrending:   noArg(a.flow.Charts),
		cmdSubscribe:  noArg(a.flow.Subscriptions),
		cmdNewlyAdded: noArg(a.flow.Latest),
		cmdUpcoming:   noArg(a.flow.Upcoming),
		cmdTasks:      noArg(a.flow.Tasks),
		cmdHelp:       func(context.Context, flow.Actor, string) flow.Reply { return flow.Reply{Text: help} },
	}
}

// start opens the home menu, or with a deep link's MediaRef that media.
func (a *adapter) start(ctx context.Context, actor flow.Actor, arg string) flow.Reply {
	if arg == "" {
		return a.flow.Home(ctx, actor)
	}
	return a.flow.Open(ctx, actor, arg)
}

// onCallback answers the query last, so the client's spinner covers the
// backend round trip and the answer can carry a notice. The owner's press
// stops the message's follower before waiting for the message: a refresh
// may be a long backend call (a resource search) holding it.
func (a *adapter) onCallback(ctx context.Context, b *bot.Bot, cq *models.CallbackQuery) {
	actor := actorOf(cq.From, callbackChat(cq))
	if a.allowed[cq.From.ID] {
		a.stopOwnedFollower(callbackKey(cq), actor.UserID)
	}
	unlock, ok := a.lockMessage(ctx, callbackKey(cq))
	if !ok {
		return
	}
	defer unlock()
	reply := flow.Reply{Notice: "抱歉，你还没有使用权限哦～"}
	if a.allowed[cq.From.ID] {
		reply = a.flow.Choose(ctx, actor, cq.Data)
	}
	if reply.Notice == "" && cq.Message.Message != nil {
		a.showCallback(ctx, cq, reply)
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

// editTarget is a conversation message and the user driving it.
type editTarget struct {
	chat, user int64
	message    int
	photo      bool
}

// edit renders reply into the conversation message and records whether it
// now waits for a typed answer.
func (a *adapter) edit(ctx context.Context, t editTarget, reply flow.Reply) (editTarget, bool) {
	shown, err := a.display(ctx, t, reply)
	if err != nil {
		a.logFailure("edit reply", err)
		return t, false
	}
	key := inputKey{chat: t.chat, user: t.user}
	a.mu.Lock()
	if reply.Input != "" {
		a.pending[key] = pendingInput{message: shown.message, input: reply.Input}
	} else if a.pending[key].message == t.message {
		delete(a.pending, key)
	}
	a.mu.Unlock()
	return shown, true
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

// parseCommand splits "/search@SomeBot dune part two" into its command and
// argument; group chats address commands to a bot with the @ suffix.
func parseCommand(text string) (cmd, arg string, ok bool) {
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	head, rest, _ := strings.Cut(text[1:], " ")
	cmd, _, _ = strings.Cut(head, "@")
	return strings.ToLower(cmd), strings.TrimSpace(rest), true
}
