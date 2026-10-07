package flow

import (
	"context"
	"fmt"
	"slices"
)

const (
	actionHistory     = "hs" // arg: Kind; opens the subscription history
	actionHistoryKind = "hf" // arg: Kind; switches the history to that kind
	actionHistoryPick = "hp" // arg: index into session past; asks to subscribe again
	actionResubscribe = "ho" // arg: index into session past; subscribes it again

	// historyCount is how many of a kind's newest past subscriptions show.
	historyCount = 10
	// dateMinutes trims MoviePilot's "2006-01-02 15:04:05" to the minute.
	dateMinutes = len("2006-01-02 15:04")

	msgHistoryTitle     = "📚 订阅历史"
	msgNoHistory        = "还没有%s订阅历史哦～"
	msgHistoryNote      = "最近 10 条，点编号可以重新订阅哦。"
	msgHistoryFailed    = "暂时查不到订阅历史，稍后再点一次试试吧～"
	msgAlreadyAgain     = "它已经在订阅中啦～"
	msgResubscribeNote  = "会沿用上次的订阅设置哦。"
	msgResubscribeLabel = "确认重新订阅"
)

// chooseHistory applies the subscription history actions; ok is false for
// others.
func (e *Engine) chooseHistory(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionHistory, actionHistoryKind:
		if !slices.Contains(subKinds, Kind(p.arg)) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		return e.history(ctx, sess, Kind(p.arg)), true
	case actionHistoryPick:
		return e.pickPast(ctx, sess, p.arg), true
	case actionResubscribe:
		return e.resubscribe(ctx, sess, p.arg), true
	}
	return Reply{}, false
}

// history lists kind's newest past subscriptions; a failed read keeps the
// screen and says so.
func (e *Engine) history(ctx context.Context, sess session, kind Kind) Reply {
	past, err := e.backend.SubscriptionHistory(ctx, kind, historyCount)
	if err != nil {
		e.log.Warn("subscription history unavailable", "kind", kind, "err", err)
		if message, safe := UserMessage(err); safe {
			return Reply{Notice: message}
		}
		return Reply{Notice: msgHistoryFailed}
	}
	sess.kind, sess.past, sess.target = kind, past, nil
	view := listView{
		heading: Heading(Plain(fmt.Sprintf("%s · %s", msgHistoryTitle, kind))),
		note:    msgHistoryNote,
		menu:    [][]Button{kindRow(sess.id, actionHistoryKind, kind)},
		footer:  append([]Button{backTo(sess.id, actionSubs, 0)}, browseRow(sess.id)...),
	}
	for i, ps := range past {
		view.entries = append(view.entries, listEntry{
			text:    Lines(pastEntry(i+1, ps)),
			buttons: []Button{{Label: fmt.Sprint(i + 1), Data: data(sess.id, actionHistoryPick, i)}},
		})
	}
	if len(past) == 0 {
		view.note = ""
		view.entries = []listEntry{{text: Sentence(fmt.Sprintf(msgNoHistory, kind))}}
	}
	return e.listPages(sess, view)
}

// pastEntry is e.g. "1. **《颂乐人偶》第 1 季**" over "_2025 · 2026-10-06 22:36_".
func pastEntry(n int, ps PastSubscription) Block {
	name := fmt.Sprintf("《%s》", truncate(ps.Media.Title, listTitleRunes))
	if ps.Season != nil {
		name += fmt.Sprintf("第 %d 季", *ps.Season)
	}
	from := ""
	if ps.StartEpisode > 1 {
		from = fmt.Sprintf("从第 %d 集", ps.StartEpisode)
	}
	return entry(n, Strong(name), ps.Media.Year, ps.Date[:min(len(ps.Date), dateMinutes)], from)
}

func pastTarget(ps PastSubscription) Target {
	return Target{Media: ps.Media, Season: ps.Season, StartEpisode: ps.StartEpisode}
}

// pickPast asks before subscribing past index again, unless it is already
// subscribed; that check is a courtesy, so a failed one still asks.
func (e *Engine) pickPast(ctx context.Context, sess session, index int) Reply {
	if index < 0 || index >= len(sess.past) {
		return Reply{Notice: msgInvalidChoice}
	}
	ps := sess.past[index]
	target := pastTarget(ps)
	id, err := e.backend.FindSubscription(ctx, target)
	if err != nil {
		e.log.Warn("subscription lookup failed", "media", ps.Media.ID, "err", err)
	}
	if err == nil && id != 0 {
		return Reply{Notice: msgAlreadyAgain}
	}
	sess.target = &target
	e.store.put(sess)
	question := Line(Strong(fmt.Sprintf("要重新订阅%s吗？", targetName(target))))
	rows := [][]Button{{{Label: msgResubscribeLabel, Data: data(sess.id, actionResubscribe, index)}}, {cancelButton(sess.id)}}
	return card{Media: ps.Media}.replyLines(Lines(question, Line(Plain(msgResubscribeNote))), rows)
}

// resubscribe writes past index again once. Steps of a session run one at
// a time and the confirmable target is cleared before the write, so a second
// tap finds nothing to confirm; the session stays for the list and home
// buttons.
func (e *Engine) resubscribe(ctx context.Context, sess session, index int) Reply {
	if index < 0 || index >= len(sess.past) || sess.target == nil || !samePick(*sess.target, sess.past[index]) {
		return Reply{Notice: msgInvalidChoice}
	}
	sess.target = nil
	e.store.put(sess)
	ps := sess.past[index]
	target := pastTarget(ps)
	id, err := e.written(target, func() (int, error) { return e.backend.Resubscribe(ctx, ps) })
	rows := [][]Button{append([]Button{backTo(sess.id, actionHistoryKind, int(sess.kind))}, browseRow(sess.id)...)}
	if err != nil {
		return card{Media: ps.Media}.replyLines(e.failure("resubscribe", err).Text, rows)
	}
	done := fmt.Sprintf("✅ 已经帮你重新订阅%s啦%s ヾ(≧▽≦*)o", targetName(target), e.watch(ctx, sess, subscription{id: id, target: target}))
	return card{Media: ps.Media}.reply(Line(Plain(done)), rows)
}

// samePick reports whether the confirmable target is past, so a button left
// from an earlier pick cannot subscribe something else.
func samePick(t Target, ps PastSubscription) bool {
	return t.Media.Source == ps.Media.Source && t.Media.ID == ps.Media.ID && seasonOf(t.Season) == seasonOf(ps.Season)
}

// noSeason stands for a movie's missing season, unlike any season number.
const noSeason = -1

func seasonOf(season *int) int {
	if season == nil {
		return noSeason
	}
	return *season
}
