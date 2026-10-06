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
	// MaxResults caps discovery matches and recommendations, not search pages.
	MaxResults = 8
	// SessionTTL is how long an untouched conversation stays answerable.
	SessionTTL = 10 * time.Minute

	actionMedia   = "m"
	actionSeason  = "s"
	actionConfirm = "ok" // arg: start episode, 0 = from the beginning
	actionAskFrom = "e"
	actionCancel  = "x"

	// seasonColumns is how many season buttons share a row.
	seasonColumns = 3
)

const (
	msgUsage         = "要在 /search 后面带上片名哦，比如 /search 沙丘～"
	msgExpired       = "⌛ 这个请求过期啦，重新 /search 一下吧～"
	msgNotYours      = "这是别人的请求哦，不能替 TA 点～"
	msgBackendDown   = "⚠️ MoviePilot 好像打了个盹，稍后再来找我吧 (´-ω-`)"
	msgCancelled     = "好哒，已经取消啦～"
	msgNoSeasons     = "呜，没查到这部剧有哪几季…"
	msgPickSeason    = "想订哪一季呀？"
	msgInvalidChoice = "这个选项不太对哦～"
	msgWillNotify    = "入库了我第一时间叫你！"
	msgNoNotice      = "MoviePilot 会自己去搜索下载哒。"
)

// Options configures an Engine.
type Options struct {
	Backend  Backend
	Calendar Calendar
	Watcher  Watcher
	Log      *slog.Logger
	Now      func() time.Time
}

// Engine runs request conversations against a Backend.
type Engine struct {
	backend  Backend
	calendar Calendar
	watcher  Watcher
	log      *slog.Logger
	now      func() time.Time
	store    *store
}

func New(opts Options) *Engine {
	return &Engine{
		backend:  opts.Backend,
		calendar: opts.Calendar,
		watcher:  opts.Watcher,
		log:      opts.Log,
		now:      opts.Now,
		store:    newStore(opts.Now, SessionTTL),
	}
}

// press is a button press: an action and its argument.
type press struct {
	action string
	arg    int
}

// chooser applies the presses it knows; ok is false for any other.
type chooser func(ctx context.Context, sess session, p press) (reply Reply, ok bool)

// Choose applies a button press.
func (e *Engine) Choose(ctx context.Context, actor Actor, raw string) Reply {
	id, p, ok := parseData(raw)
	if !ok {
		return Reply{Notice: msgInvalidChoice}
	}
	sess, unlock, ok := e.lockSession(ctx, actor, id)
	defer unlock()
	if !ok {
		return expired(p.action)
	}
	if sess.owner.UserID != actor.UserID || sess.owner.Address != actor.Address {
		return Reply{Notice: msgNotYours}
	}
	switch p.action {
	case actionPage:
		return e.page(sess, p.arg)
	case actionBack:
		return e.back(ctx, sess)
	}
	for _, choose := range []chooser{e.chooseHome, e.chooseRequest, e.chooseSeasons, e.chooseTask, e.chooseChart, e.chooseSubs, e.chooseRelated} {
		if reply, ok := choose(ctx, sess, p); ok {
			return e.navigate(sess, p.action, reply)
		}
	}
	return Reply{Notice: msgInvalidChoice}
}

// chooseRequest applies the steps of a /search conversation.
func (e *Engine) chooseRequest(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionMedia:
		return e.pickMedia(ctx, sess, p.arg), true
	case actionSeason:
		return e.pickSeason(ctx, sess, p.arg), true
	case actionConfirm:
		return e.confirm(ctx, sess, p.arg), true
	case actionAskFrom:
		return askStart(sess, ""), true
	case actionResearch:
		return askResearch(sess), true
	case actionRetry:
		return e.retryRead(ctx, sess), true
	case actionCancel:
		return e.cancel(sess), true
	}
	return Reply{}, false
}

// cancel ends the conversation; 返回 is the way back.
func (e *Engine) cancel(sess session) Reply {
	e.store.take(sess.id)
	return Reply{Text: Sentence(msgCancelled)}
}

func (e *Engine) pickMedia(ctx context.Context, sess session, index int) Reply {
	if index < 0 || index >= len(sess.results) {
		return Reply{Notice: msgInvalidChoice}
	}
	media := sess.results[index]
	sess.target, sess.seasons, sess.chosen, sess.retry = nil, nil, nil, recovery{}
	e.enrich(ctx, &sess, media)
	if media.Kind == Movie && sess.library.Movie {
		return e.held(sess, Target{Media: media})
	}
	if media.Kind == Movie {
		return e.offerConfirm(ctx, sess, Target{Media: media})
	}
	return e.offerSeasons(ctx, sess)
}

func (e *Engine) offerSeasons(ctx context.Context, sess session) Reply {
	seasons, err := e.backend.Seasons(ctx, sess.picked.Media)
	if err != nil {
		return e.readFailure(sess, recovery{step: readSeasons}, err)
	}
	if len(seasons) == 0 {
		return e.readFailure(sess, recovery{step: readSeasons}, &UserError{Message: msgNoSeasons})
	}
	sess.seasons, sess.retry = seasons, recovery{}
	e.store.put(sess)
	if len(seasons) == 1 && seasons[0].Number > 0 {
		return e.pickSeason(ctx, sess, seasons[0].Number)
	}
	picks := make([]Button, 0, len(seasons))
	for _, s := range seasons {
		picks = append(picks, Button{Label: seasonName(s.Number), Data: data(sess.id, actionSeason, s.Number)})
	}
	rows := grid(picks, seasonColumns)
	if len(sess.selectable()) > 1 {
		rows = append(rows, []Button{{Label: "多选季…", Data: data(sess.id, actionMulti, 0)}})
	}
	rows = append(rows, relatedRow(sess), []Button{cancelButton(sess.id)})
	return sess.picked.replyLines(sess.seasonTable(msgPickSeason, seasons), rows)
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

// library tells what the media server already holds; like details it only
// informs the conversation. A failed read is marked unknown on the card.
func (e *Engine) library(ctx context.Context, media Media) (Library, error) {
	l, err := e.backend.Library(ctx, media)
	if err != nil {
		e.log.Warn("library check unavailable", "media", media.ID, "err", err)
	}
	return l, err
}

// downloads lists target's unfinished downloads; like library it only
// informs, so a failure is logged and shows nothing.
func (e *Engine) downloads(ctx context.Context, target Target) []Download {
	all, err := e.backend.Downloads(ctx)
	if err != nil {
		e.log.Warn("downloads unavailable", "media", target.Media.ID, "err", err)
	}
	var out []Download
	for _, d := range all {
		if target.has(d) {
			out = append(out, d)
		}
	}
	return out
}

// held ends the request: what was asked for is already watchable. The
// session stays only to browse on from it.
func (e *Engine) held(sess session, target Target) Reply {
	e.store.put(sess)
	what := "已经在媒体库里啦"
	if target.Season != nil {
		what = "已经全部在媒体库里啦"
	}
	return sess.picked.reply(Line(Plain(fmt.Sprintf("✅ %s%s，直接去看吧～", targetName(target), what))), [][]Button{relatedRow(sess)})
}

func (e *Engine) pickSeason(ctx context.Context, sess session, number int) Reply {
	for _, s := range sess.seasons {
		if s.Number != number {
			continue
		}
		target := Target{Media: sess.picked.Media, Season: &number}
		if sess.wholeSeasonHeld(s) {
			return e.held(sess, target)
		}
		return e.offerConfirm(ctx, sess, target)
	}
	return Reply{Notice: msgInvalidChoice}
}

// offerConfirm ends early when the target is already subscribed, otherwise
// asks for confirmation.
func (e *Engine) offerConfirm(ctx context.Context, sess session, target Target) Reply {
	existing, err := e.confirmationInfo(ctx, &sess, target)
	if err != nil {
		return e.readFailure(sess, recovery{step: readSubscription, target: target}, err)
	}
	return e.confirmation(ctx, sess, subscription{id: existing, target: target})
}

func (e *Engine) confirmation(ctx context.Context, sess session, sub subscription) Reply {
	sess.retry, sess.target = recovery{}, nil
	target := sub.target
	if sub.id != 0 {
		e.store.put(sess)
		status := fmt.Sprintf("ℹ️ %s早就订阅上啦%s", targetName(target), e.watch(ctx, sess, sub))
		return sess.picked.reply(Line(Plain(status)), [][]Button{relatedRow(sess)})
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
	question := Line(Strong(fmt.Sprintf("要订阅%s吗？", targetName(target))))
	rows := [][]Button{{confirm, cancelButton(sess.id)}}
	if target.Season == nil {
		rows = append(rows, relatedRow(sess))
	}
	return sess.picked.reply(question, rows)
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
	id, err := e.submit(ctx, target)
	if err != nil {
		return sess.picked.replyLines(e.failure("subscribe", err).Text, nil)
	}
	done := fmt.Sprintf("✅ 帮你订好%s啦%s ヾ(≧▽≦*)o", targetName(target), e.watch(ctx, sess, subscription{id: id, target: target}))
	return sess.picked.reply(Line(Plain(done)), nil)
}

// subscription is a MoviePilot subscription and what it was made for.
type subscription struct {
	id     int
	target Target
}

// watch registers the session owner for an arrival notice and returns the
// sentence ending that tells them whether they will get one.
func (e *Engine) watch(ctx context.Context, sess session, sub subscription) string {
	if !e.remember(ctx, sess, sub) {
		return "，" + msgNoNotice
	}
	return "，" + msgWillNotify
}

// remember registers the session owner for an arrival notice of sub.
func (e *Engine) remember(ctx context.Context, sess session, sub subscription) bool {
	req := Request{SubscriptionID: sub.id, Target: sub.target, Requester: sess.owner}
	if season := sub.target.Season; season != nil {
		req.SeasonEpisodes = sess.episodeCount(*season)
		req.Held = sess.library.Episodes[*season]
	}
	if err := e.watcher.Watch(ctx, req); err != nil {
		e.log.Error("cannot remember request for notification", "subscription", sub.id, "err", err)
		return false
	}
	return true
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

func parseData(raw string) (id uint64, p press, ok bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return 0, press{}, false
	}
	id, errID := strconv.ParseUint(parts[0], 10, 64)
	arg, errArg := strconv.Atoi(parts[2])
	return id, press{action: parts[1], arg: arg}, errID == nil && errArg == nil
}

func cancelButton(id uint64) Button {
	return Button{Label: "取消", Data: data(id, actionCancel, 0)}
}
