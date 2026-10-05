package flow

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const (
	// MaxResults caps how many search results become buttons.
	MaxResults = 8
	// SessionTTL is how long an untouched conversation stays answerable.
	SessionTTL = 10 * time.Minute

	actionMedia   = "m"
	actionSeason  = "s"
	actionConfirm = "ok" // arg: start episode, 0 = from the beginning
	actionAskFrom = "e"
	actionCancel  = "x"
)

const (
	msgUsage         = "用法：/request <片名>"
	msgExpired       = "⌛ 这个请求已失效，请重新 /request。"
	msgNotYours      = "这不是你发起的请求。"
	msgBackendDown   = "⚠️ MoviePilot 暂时不可用，请稍后再试。"
	msgCancelled     = "已取消。"
	msgNoSeasons     = "没有查到这部剧的季信息。"
	msgPickSeason    = "选择要订阅的季："
	msgInvalidChoice = "无效的选项。"
)

// Options configures an Engine.
type Options struct {
	Backend Backend
	Log     *slog.Logger
	Now     func() time.Time
}

// Engine runs request conversations against a Backend.
type Engine struct {
	backend Backend
	log     *slog.Logger
	store   *store
}

func New(opts Options) *Engine {
	return &Engine{
		backend: opts.Backend,
		log:     opts.Log,
		store:   newStore(opts.Now, SessionTTL),
	}
}

// Start searches for term and offers the results.
func (e *Engine) Start(ctx context.Context, actor Actor, term string) Reply {
	term = strings.TrimSpace(term)
	if term == "" {
		return Reply{Text: Sentence(msgUsage)}
	}
	results, err := e.backend.Search(ctx, term)
	if err != nil {
		return e.failure("search", err)
	}
	if len(results) == 0 {
		return Reply{Text: Sentence(fmt.Sprintf("🔍 没有找到「%s」相关的影视。", term))}
	}
	if len(results) > MaxResults {
		results = results[:MaxResults]
	}
	sess := e.store.create(actor.UserID, results)
	text := make(Text, 0, len(results)+1)
	text = append(text, Line(Strong(fmt.Sprintf("🔍「%s」的搜索结果", term))))
	rows := make([][]Button, 0, len(results)+1)
	for i, m := range results {
		text = append(text, resultLine(i+1, m))
		label := fmt.Sprintf("%d. %s", i+1, titleYear(m))
		rows = append(rows, []Button{{Label: label, Data: data(sess.id, actionMedia, i)}})
	}
	rows = append(rows, []Button{cancelButton(sess.id)})
	return Reply{Text: text, Buttons: rows}
}

// Choose applies a button press.
func (e *Engine) Choose(ctx context.Context, actor Actor, raw string) Reply {
	id, action, arg, ok := parseData(raw)
	if !ok {
		return Reply{Notice: msgInvalidChoice}
	}
	sess, ok := e.store.get(id)
	if !ok {
		return Reply{Text: Sentence(msgExpired)}
	}
	if sess.owner != actor.UserID {
		return Reply{Notice: msgNotYours}
	}
	switch action {
	case actionMedia:
		return e.pickMedia(ctx, sess, arg)
	case actionSeason:
		return e.pickSeason(ctx, sess, arg)
	case actionConfirm:
		return e.confirm(ctx, sess, arg)
	case actionAskFrom:
		return askStart(sess, "")
	case actionCancel:
		e.store.take(id)
		return Reply{Text: Sentence(msgCancelled)}
	}
	return Reply{Notice: msgInvalidChoice}
}

func (e *Engine) pickMedia(ctx context.Context, sess session, index int) Reply {
	if index < 0 || index >= len(sess.results) {
		return Reply{Notice: msgInvalidChoice}
	}
	sess.picked = card{Media: sess.results[index], Details: e.details(ctx, sess.results[index])}
	if sess.picked.Media.Kind == Movie {
		return e.offerConfirm(ctx, sess, Target{Media: sess.picked.Media})
	}
	seasons, err := e.backend.Seasons(ctx, sess.picked.Media)
	if err != nil {
		e.store.take(sess.id)
		return e.failure("seasons", err)
	}
	if len(seasons) == 0 {
		e.store.take(sess.id)
		return sess.picked.reply(Line(Plain(msgNoSeasons)), nil)
	}
	sess.seasons = seasons
	e.store.put(sess)
	rows := make([][]Button, 0, len(seasons)+1)
	for _, s := range seasons {
		rows = append(rows, []Button{{Label: seasonLabel(s), Data: data(sess.id, actionSeason, s.Number)}})
	}
	rows = append(rows, []Button{cancelButton(sess.id)})
	return sess.picked.reply(Line(Strong(msgPickSeason)), rows)
}

// details enriches the card; it is cosmetic, so a failure is logged and the
// conversation goes on with the search metadata alone.
func (e *Engine) details(ctx context.Context, media Media) Details {
	d, err := e.backend.Details(ctx, media)
	if err != nil {
		e.log.Warn("media details unavailable", "media", media.ID, "err", err)
	}
	return d
}

func (e *Engine) pickSeason(ctx context.Context, sess session, number int) Reply {
	for _, s := range sess.seasons {
		if s.Number == number {
			return e.offerConfirm(ctx, sess, Target{Media: sess.picked.Media, Season: &number})
		}
	}
	return Reply{Notice: msgInvalidChoice}
}

// offerConfirm ends early when the target is already subscribed, otherwise
// asks for confirmation.
func (e *Engine) offerConfirm(ctx context.Context, sess session, target Target) Reply {
	subscribed, err := e.backend.IsSubscribed(ctx, target)
	if err != nil {
		e.store.take(sess.id)
		return e.failure("subscription lookup", err)
	}
	if subscribed {
		e.store.take(sess.id)
		return sess.picked.reply(Line(Plain(fmt.Sprintf("ℹ️ %s已在订阅中，无需重复请求。", targetName(target)))), nil)
	}
	sess.target = &target
	e.store.put(sess)
	if target.Season != nil {
		return startChoices(sess, *target.Season)
	}
	return confirmCard(sess, target)
}

// confirmCard asks for the final go-ahead on a fully specified target.
func confirmCard(sess session, target Target) Reply {
	confirm := Button{Label: "确认订阅", Data: data(sess.id, actionConfirm, target.StartEpisode)}
	question := Line(Strong(fmt.Sprintf("确认订阅%s？", targetName(target))))
	return sess.picked.reply(question, [][]Button{{confirm, cancelButton(sess.id)}})
}

// confirm subscribes from episode from (0 for movies or the season start).
// The session is taken only after validation, so a forged or stale start
// cannot consume it.
func (e *Engine) confirm(ctx context.Context, sess session, from int) Reply {
	if sess.target == nil {
		return Reply{Text: Sentence(msgExpired)}
	}
	if !sess.validStart(from) {
		return Reply{Notice: msgInvalidChoice}
	}
	if _, ok := e.store.take(sess.id); !ok {
		return Reply{Text: Sentence(msgExpired)}
	}
	target := *sess.target
	target.StartEpisode = from
	if err := e.backend.Subscribe(ctx, target); err != nil {
		return e.failure("subscribe", err)
	}
	done := fmt.Sprintf("✅ 已订阅%s，MoviePilot 会自动搜索下载。", targetName(target))
	return sess.picked.reply(Line(Plain(done)), nil)
}

func (e *Engine) failure(step string, err error) Reply {
	if msg, ok := UserMessage(err); ok {
		return Reply{Text: Sentence("⚠️ " + msg)}
	}
	e.log.Error("backend call failed", "step", step, "err", err)
	return Reply{Text: Sentence(msgBackendDown)}
}

func data(id uint64, action string, arg int) string {
	return fmt.Sprintf("%d:%s:%d", id, action, arg)
}

func parseData(raw string) (id uint64, action string, arg int, ok bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return 0, "", 0, false
	}
	id, errID := strconv.ParseUint(parts[0], 10, 64)
	arg, errArg := strconv.Atoi(parts[2])
	return id, parts[1], arg, errID == nil && errArg == nil
}

func cancelButton(id uint64) Button {
	return Button{Label: "取消", Data: data(id, actionCancel, 0)}
}
