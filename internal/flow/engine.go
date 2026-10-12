package flow

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
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
	msgUsage       = "要在 /search 后面带上片名哦，比如 /search 沙丘～"
	msgExpired     = "⌛ 这个请求过期啦，重新 /search 一下吧～"
	msgNotYours    = "这是别人的请求哦，不能替 TA 点～"
	msgBackendDown = "⚠️ MoviePilot 好像打了个盹，稍后再来找我吧 (´-ω-`)"
	// msgBackendUnavailable is the emoji-free, kaomoji-free form for toasts and table cells.
	msgBackendUnavailable = "MoviePilot 暂时没有响应，稍后再试吧"
	msgCancelled          = "好哒，已经取消啦～"
	msgNoSeasons          = "呜，没查到这部剧有哪几季…"
	msgPickSeason         = "想订哪一季呀？"
	msgInvalidChoice      = "这个选项不太对哦～"
	msgWillNotify         = "通知由 MoviePilot 按设置发送哦～"
)

// Options configures an Engine.
type Options struct {
	RunContext context.Context
	Backend    Backend
	Calendar   Calendar
	Log        *slog.Logger
	Now        func() time.Time
}

// Engine runs request conversations against a Backend.
type Engine struct {
	runCtx   context.Context
	work     sync.WaitGroup
	backend  Backend
	calendar Calendar
	log      *slog.Logger
	now      func() time.Time
	store    *store
}

func New(opts Options) *Engine {
	return &Engine{
		runCtx:   opts.RunContext,
		backend:  opts.Backend,
		calendar: opts.Calendar,
		log:      opts.Log,
		now:      opts.Now,
		store:    newStore(opts.Now, SessionTTL),
	}
}

// Wait is called after platform handlers stop starting work during shutdown.
func (e *Engine) Wait() { e.work.Wait() }

// press is a button press: an action and its argument.
type press struct {
	action string
	arg    int
	ticket uint64 // a write button's ticket (ticketed); 0 on other buttons
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
		if reply, revived := e.revive(ctx, actor, p); revived {
			return reply
		}
		return expired(p.action)
	}
	if sess.owner.UserID != actor.UserID || sess.owner.Address != actor.Address {
		return Reply{Notice: msgNotYours}
	}
	if sess.lastChoice == raw {
		return sess.lastReply
	}
	if !availableChoice(sess, raw, p.action) {
		return Reply{Notice: msgInvalidChoice}
	}
	if needsTicket(p.action) && (p.ticket == 0 || p.ticket != sess.ticket) {
		return Reply{Notice: msgInvalidChoice}
	}
	reply := e.choose(ctx, sess, p)
	e.rememberChoice(sess.id, raw, reply)
	return reply
}

// Keep only the latest accepted read's rendered result. A repeat can
// recover an undelivered edit without repeating backend calls or history.
// Any later transition invalidates it; writes are never replayed here.
func (e *Engine) rememberChoice(id uint64, raw string, reply Reply) {
	if reply.Notice != "" {
		return
	}
	sess, ok := e.store.get(id)
	if !ok {
		return
	}
	sess.lastChoice, sess.lastReply = "", Reply{}
	_, p, _ := parseData(raw)
	if forward[p.action] && p.action != actionTorrentRun && !needsTicket(p.action) {
		sess.lastChoice, sess.lastReply = raw, reply
	}
	e.store.put(sess)
}

// A terminal receipt is immutable. Forward choices must still belong to
// the shown screen, so an already accepted list pick cannot run twice.
func availableChoice(sess session, raw, action string) bool {
	if sess.submission != nil {
		return action == actionBatchProgress || action == actionBatchStop
	}
	if sess.searchView == torrentWaitingView {
		return waitingTorrentChoice(action)
	}
	if allowed, handled := planScreenChoice(sess, raw, action); handled {
		return allowed
	}
	if allowed, handled := subSearchChoice(sess, raw, action); handled {
		return allowed
	}
	return availableScreenChoice(sess, raw, action)
}

func availableScreenChoice(sess session, raw, action string) bool {
	shown := sess.screen.reply
	if subEditAction(action) || sess.subEdit != nil {
		return hasData(shown.Buttons, raw)
	}
	if allowed, handled := availableTorrentChoice(sess, raw, action); handled {
		return allowed
	}
	if sess.screen.state != nil && len(shown.Buttons) == 0 && shown.Follow == "" {
		return false
	}
	if forward[action] && action != actionTorrentRun {
		return hasData(shown.Buttons, raw)
	}
	return true
}

func waitingTorrentChoice(action string) bool {
	return action == actionCancel || action == actionTorrentRun || action == actionTorrentAgain
}

func availableTorrentChoice(sess session, raw, action string) (bool, bool) {
	shown := sess.screen.reply
	if startsTorrentSearch(action) {
		return sess.searchView != torrentWaitingView && hasData(shown.Buttons, raw), true
	}
	if sess.streaming && torrentAction(action) {
		if action == actionTorrentRun || action == actionTorrentAgain {
			return sess.searchView == torrentWaitingView || shown.Follow == raw, true
		}
		return hasData(shown.Buttons, raw), true
	}
	if action == actionSearchSite || action == actionSearchAll {
		return hasData(shown.Buttons, raw), true
	}
	return false, false
}

func torrentAction(action string) bool {
	return torrentScreenAction(action) && action != actionBack && action != actionPage
}

func startsTorrentSearch(action string) bool {
	return action == actionTorrents || action == actionTorrentRetry || action == actionTorrentResearch || action == actionSearchStart
}

// choose dispatches a press only after Choose has checked its owner and ticket.
func (e *Engine) choose(ctx context.Context, sess session, p press) Reply {
	switch p.action {
	case actionPage:
		return e.page(sess, p.arg)
	case actionBack:
		if reply, handled := e.backFromSubEdit(sess); handled {
			return reply
		}
		return e.back(ctx, sess)
	case actionBatchProgress, actionBatchStop:
		return e.batchProgress(sess, p)
	case actionTorrentBatchGet:
		return e.confirmTorrentPlan(ctx, sess)
	case actionCancel:
		if sess.subEdit != nil && sess.subEdit.dirty() {
			return e.navigate(sess, actionSubEditApply, e.askSubEditLeave(sess, actionCancel))
		}
	}
	return e.chooseAction(ctx, sess, p)
}

func (e *Engine) chooseAction(ctx context.Context, sess session, p press) Reply {
	for _, choose := range []chooser{e.chooseHome, e.chooseRequest, e.chooseSeasons, e.chooseTask, e.chooseChart, e.chooseSubs, e.chooseSubDetail, e.chooseSubPause, e.chooseSubEdit, e.chooseSubSearch, e.chooseHistory, e.chooseRelated, e.choosePeople, e.chooseFilter, e.chooseGroups, e.chooseSeriesPlan, e.chooseTorrents, e.chooseSearchSites, e.chooseTorrentView, e.chooseTorrentFilter, e.chooseTorrentBatch, e.chooseTorrentPlan, e.chooseDelete, e.chooseToggle, e.chooseUpgrade, e.chooseCategory} {
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
	sess.hunt.stop()
	e.store.take(sess.id)
	return Reply{Text: Lines(Heading(Plain("已取消")), Line(Plain(msgCancelled)))}
}

func (e *Engine) pickMedia(ctx context.Context, sess session, index int) Reply {
	if index < 0 || index >= len(sess.results) {
		return Reply{Notice: msgInvalidChoice}
	}
	media := sess.results[index]
	sess.target, sess.seasons, sess.chosen, sess.retry, sess.focus = nil, nil, nil, recovery{}, nil
	sess.group, sess.groups = EpisodeGroup{}, nil
	if media.Kind == Movie {
		e.enrich(ctx, &sess, media)
		if sess.library.Movie {
			return e.held(sess, Target{Media: media})
		}
		return e.offerConfirm(ctx, sess, Target{Media: media})
	}
	// A show's seasons are read alongside the card's metadata: both wait
	// on TMDB through MoviePilot, so the pick costs one round trip.
	var seasons []Season
	var err error
	var wg sync.WaitGroup
	wg.Go(func() { seasons, err = e.backend.Seasons(ctx, media) })
	e.enrich(ctx, &sess, media)
	wg.Wait()
	return e.showSeasons(ctx, sess, seasonRead{seasons, err})
}

func (e *Engine) offerSeasons(ctx context.Context, sess session) Reply {
	seasons, err := e.backend.Seasons(ctx, sess.picked.Media)
	return e.showSeasons(ctx, sess, seasonRead{seasons, err})
}

type seasonRead struct {
	seasons []Season
	err     error
}

// showSeasons offers the seasons just read, or the read's failure.
func (e *Engine) showSeasons(ctx context.Context, sess session, read seasonRead) Reply {
	seasons, err := read.seasons, read.err
	if err != nil {
		return e.readFailure(sess, recovery{step: readSeasons}, err)
	}
	if len(seasons) == 0 {
		return e.readFailure(sess, recovery{step: readSeasons}, &UserError{Message: msgNoSeasons})
	}
	sess.seasons, sess.retry = seasons, recovery{}
	e.store.put(sess)
	if n, ok := impliedSeason(sess.picked.Media, seasons); ok && sess.group.ID == "" {
		return e.pickSeason(ctx, sess, n)
	}
	picks := make([]Button, 0, len(seasons))
	for _, s := range seasons {
		picks = append(picks, Button{Label: seasonName(s.Number), Data: data(sess.id, actionSeason, s.Number)})
	}
	rows := grid(picks, seasonColumns)
	if len(sess.selectable()) > 1 && sess.group.ID == "" {
		rows = append(rows, []Button{{Label: "多选季…", Data: data(sess.id, actionMulti, 0)}})
	}
	if groupsOffered(sess) {
		rows = append(rows, []Button{{Label: labelGroups, Data: data(sess.id, actionGroups, 0)}})
	}
	rows = append(rows, relatedRow(sess), []Button{cancelButton(sess.id)})
	question := msgPickSeason
	if sess.group.ID != "" {
		question = fmt.Sprintf("按剧集组「%s」的编排，%s", sess.group.Name, msgPickSeason)
	}
	return sess.picked.replyLines(sess.seasonTable(question, seasons), rows)
}

// impliedSeason is the season a show's pick already names: the one its
// search term named (第二季) when the show has it, else its sole season
// unless that is the specials. The confirmation still follows.
func impliedSeason(media Media, seasons []Season) (int, bool) {
	if media.Season > 0 && slices.ContainsFunc(seasons, func(s Season) bool { return s.Number == media.Season }) {
		return media.Season, true
	}
	if len(seasons) == 1 && seasons[0].Number > 0 {
		return seasons[0].Number, true
	}
	return 0, false
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
// session stays only to browse on from it, or for a title searched with
// 下载 searches its resources at once all the same.
func (e *Engine) held(sess session, target Target) Reply {
	sess.focus = &target
	e.store.put(sess)
	what := "已经在媒体库里啦"
	if target.Season != nil {
		what = "已经全部在媒体库里啦"
	}
	rows := append([][]Button{upgradeRow(sess)}, onward(sess)...)
	card := sess.picked.reply(Line(Plain(fmt.Sprintf("✅ %s%s，直接去看吧～", targetName(target), what))), rows)
	if sess.download {
		return e.huntNow(sess, card)
	}
	return card
}

func (e *Engine) pickSeason(ctx context.Context, sess session, number int) Reply {
	for _, s := range sess.seasons {
		if s.Number != number {
			continue
		}
		target := Target{Media: sess.picked.Media, Season: &number, EpisodeGroup: sess.group.ID, GroupName: sess.group.Name}
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
	return e.confirmation(sess, subscription{id: existing.ID, state: existing.State, target: target})
}

// confirmation shows the card for sub's target, or for a title searched
// with 下载 starts searching its resources at once.
func (e *Engine) confirmation(sess session, sub subscription) Reply {
	sess, card := e.targetCard(sess, sub)
	if sess.download {
		return e.huntNow(sess, card)
	}
	return card
}

// targetCard says sub's target is already subscribed, or asks to confirm
// it; the session returned is the one stored for the card.
func (e *Engine) targetCard(sess session, sub subscription) (session, Reply) {
	sess.retry, sess.target = recovery{}, nil
	target := sub.target
	sess.focus = &target
	if sub.id != 0 {
		e.store.put(sess)
		state := "早就订阅上啦"
		if sub.state == statePaused {
			state = "已订阅，目前已暂停哦"
		}
		status := fmt.Sprintf("ℹ️ %s%s%s", targetName(target), state, noticeEnding)
		return sess, sess.picked.reply(Callout(Plain(status)), onward(sess))
	}
	sess.target = &target
	e.store.put(sess)
	if target.Season != nil {
		return sess, startChoices(sess, *target.Season)
	}
	return sess, confirmCard(sess, target)
}

// confirmCard asks for the final go-ahead on a fully specified target.
func confirmCard(sess session, target Target) Reply {
	confirm := Button{Label: "确认订阅", Data: data(sess.id, actionConfirm, target.StartEpisode), Style: StylePrimary}
	question := Line(Strong(fmt.Sprintf("要订阅%s吗？", targetName(target))))
	choices := categoryRow(sess)
	if target.Season == nil {
		choices = append(choices, upgradeRow(sess)...) // a season's upgrade is offered with its start choices
	}
	rows := [][]Button{{confirm}, choices}
	return sess.picked.replyLines(append(Lines(question), categoryFact(target)...), append(rows, onward(sess)...))
}

// confirm subscribes from episode from (0 for movies or the season start).
// The request is settled only after validation, so a forged or stale start
// cannot consume it.
func (e *Engine) confirm(ctx context.Context, sess session, from int) Reply {
	if sess.target == nil || sess.target.BestVersion || !sess.validStart(from) {
		return Reply{Notice: msgInvalidChoice}
	}
	e.settle(sess)
	target := *sess.target
	target.StartEpisode = from
	if _, err := e.submit(ctx, target); err != nil {
		return sess.picked.replyLines(e.failure("subscribe", err).Text, nil)
	}
	done := fmt.Sprintf("✅ 帮你订好%s啦%s ヾ(≧▽≦*)o", targetName(target), noticeEnding)
	return sess.picked.reply(Line(Plain(done)), nil)
}

// subscription is a MoviePilot subscription and what it was made for.
type subscription struct {
	id     int
	state  string
	target Target
}

// noticeEnding closes a write receipt: MoviePilot decides every notification.
const noticeEnding = "，" + msgWillNotify

func (e *Engine) failure(step string, err error) Reply {
	if msg, ok := UserMessage(err); ok {
		return Reply{Text: Lines(Callout(Plain("⚠️ " + msg)))}
	}
	e.log.Error("backend call failed", "step", step, "err", err)
	return Reply{Text: Lines(Callout(Plain(msgBackendDown)))}
}

func data(id uint64, action string, arg int) string {
	return fmt.Sprintf("%d:%s:%d", id, action, arg)
}

func parseData(raw string) (id uint64, p press, ok bool) {
	parts := strings.Split(raw, ":")
	if len(parts) == ticketedParts {
		ticket, err := strconv.ParseUint(parts[3], 10, 64)
		p.ticket, parts = ticket, parts[:3]
		if err != nil {
			return 0, press{}, false
		}
	}
	if len(parts) != 3 {
		return 0, press{}, false
	}
	id, errID := strconv.ParseUint(parts[0], 10, 64)
	arg, errArg := strconv.Atoi(parts[2])
	p.action, p.arg = parts[1], arg
	return id, p, errID == nil && errArg == nil
}

// onward are a card's last rows: the resource search (搜索资源 beside
// 选择搜索站点), then browsing on (相似推荐, 同系列, 演职人员), then 取消.
func onward(sess session) [][]Button {
	var rows [][]Button
	for _, row := range [][]Button{searchRow(sess), browseOnRow(sess)} {
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	return append(rows, []Button{cancelButton(sess.id)})
}

func cancelButton(id uint64) Button {
	return Button{Label: "取消", Data: data(id, actionCancel, 0)}
}
