package flow

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

const (
	actionHistory     = "hs" // arg: Kind; opens the subscription history
	actionHistoryKind = "hf" // arg: Kind; switches the history to that kind
	actionHistoryMore = "hm" // reads the history's next upstream page
	actionHistoryPick = "hp" // arg: index into session past; asks to subscribe again
	actionResubscribe = "ho" // arg: index into session past; subscribes it again

	// historyCount is how many past subscriptions one upstream page holds.
	historyCount = 10
	// dateMinutes trims MoviePilot's "2006-01-02 15:04:05" to the minute.
	dateMinutes = len("2006-01-02 15:04")

	msgHistoryTitle     = "📚 订阅历史"
	msgNoHistory        = "还没有%s订阅历史哦～"
	msgHistoryPick      = "点编号可以重新订阅"
	msgHistoryMoreHint  = "，点「更多历史」看更早的"
	msgHistoryFailed    = "暂时查不到订阅历史，稍后再点一次试试吧～"
	msgAlreadyAgain     = "它已经在订阅中啦～"
	msgResubscribeNote  = "会沿用上次的订阅设置哦。"
	msgResubscribeLabel = "确认重新订阅"
	labelHistoryMore    = "更多历史"
)

// pastPaging is how far the shown subscription history has been read.
type pastPaging struct {
	page     int   // upstream pages read
	more     bool  // the last page read was full and short of the total
	total    int   // the backend's latest count of the kind's history, -1 when unknown
	unlisted []int // ids of records read without a media identity
}

// read is p once page number n has been read.
func (p pastPaging) read(n int, page HistoryPage) pastPaging {
	unlisted := slices.Clone(p.unlisted)
	for _, id := range page.Unlisted {
		if !slices.Contains(unlisted, id) {
			unlisted = append(unlisted, id)
		}
	}
	full := len(page.Past)+len(page.Unlisted) >= historyCount
	return pastPaging{
		page: n, total: page.Total, unlisted: unlisted,
		more: full && (page.Total < 0 || n*historyCount < page.Total),
	}
}

// chooseHistory applies the subscription history actions; ok is false for
// others.
func (e *Engine) chooseHistory(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionHistory, actionHistoryKind:
		if !slices.Contains(subKinds, Kind(p.arg)) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		return e.history(ctx, sess, Kind(p.arg)), true
	case actionHistoryMore:
		return e.moreHistory(ctx, sess), true
	case actionHistoryPick:
		return e.pickPast(ctx, sess, p.arg), true
	case actionResubscribe:
		return e.resubscribe(ctx, sess, p.arg), true
	}
	return Reply{}, false
}

// history lists the first page of kind's past subscriptions, newest first;
// a failed read keeps the screen and says so.
func (e *Engine) history(ctx context.Context, sess session, kind Kind) Reply {
	page, err := e.backend.SubscriptionHistory(ctx, HistoryQuery{Kind: kind, Page: 1, Count: historyCount})
	if err != nil {
		return e.historyFailed(kind, err)
	}
	sess.kind, sess.past, sess.target = kind, page.Past, nil
	sess.pastPages = pastPaging{}.read(1, page)
	return e.listPages(sess, historyView(sess))
}

// moreHistory appends the next upstream page and shows the page holding its
// first new record. Records listed already keep their numbers; one that a
// newly finished subscription pushed onto the next page is not listed twice.
func (e *Engine) moreHistory(ctx context.Context, sess session) Reply {
	if !sess.pastPages.more {
		return Reply{Notice: msgInvalidChoice}
	}
	next := sess.pastPages.page + 1
	page, err := e.backend.SubscriptionHistory(ctx, HistoryQuery{Kind: sess.kind, Page: next, Count: historyCount})
	if err != nil {
		return e.historyFailed(sess.kind, err)
	}
	first := len(sess.past)
	past := slices.Clone(sess.past)
	for _, ps := range page.Past {
		if !slices.ContainsFunc(past, func(p PastSubscription) bool { return p.ID == ps.ID }) {
			past = append(past, ps)
		}
	}
	sess.past, sess.pastPages = past, sess.pastPages.read(next, page)
	// With nothing new, the page of the last record listed stays in view.
	holds := data(sess.id, actionHistoryPick, min(first, len(past)-1))
	return e.listPageWith(sess, historyView(sess), holds)
}

func (e *Engine) historyFailed(kind Kind, err error) Reply {
	e.log.Warn("subscription history unavailable", "kind", kind, "err", err)
	if message, safe := UserMessage(err); safe {
		return Reply{Notice: message}
	}
	return Reply{Notice: msgHistoryFailed}
}

// historyView lists the history read so far, numbered in reading order.
func historyView(sess session) listView {
	view := listView{
		heading: Heading(Plain(fmt.Sprintf("%s · %s", msgHistoryTitle, sess.kind))),
		note:    historyNote(sess),
		menu:    [][]Button{kindRow(sess.id, actionHistoryKind, sess.kind)},
		footer:  append([]Button{backTo(sess.id, actionSubs, 0)}, browseRow(sess.id)...),
	}
	if sess.pastPages.more {
		view.menu = slices.Insert(view.menu, 0, []Button{{Label: labelHistoryMore, Data: data(sess.id, actionHistoryMore, 0)}})
	}
	for i, ps := range sess.past {
		view.entries = append(view.entries, listEntry{
			text:    Lines(pastEntry(i+1, ps)),
			buttons: []Button{{Label: fmt.Sprint(i + 1), Data: data(sess.id, actionHistoryPick, i)}},
		})
	}
	if len(sess.past) == 0 && view.note == "" {
		view.entries = []listEntry{{text: Sentence(fmt.Sprintf(msgNoHistory, sess.kind))}}
	}
	return view
}

// historyNote is e.g. "共 12 条，已列出 10 条。点编号可以重新订阅，点「更多历史」
// 看更早的哦。"; empty for a history with nothing in it.
func historyNote(sess session) string {
	p, listed := sess.pastPages, len(sess.past)
	if listed == 0 && len(p.unlisted) == 0 && !p.more {
		return ""
	}
	hint := ""
	if listed > 0 {
		hint = msgHistoryPick
	}
	if p.more {
		hint += msgHistoryMoreHint
	}
	if hint != "" {
		hint = strings.TrimPrefix(hint, "，") + "哦。"
	}
	return historyCounts(p, listed) + hint
}

// historyCounts is e.g. "共 12 条，已列出 9 条（1 条缺少媒体信息）。", the listed
// count shown while it differs from the total or more may follow.
func historyCounts(p pastPaging, listed int) string {
	var counts []string
	if p.total >= 0 {
		counts = append(counts, fmt.Sprintf("共 %d 条", p.total))
	}
	if p.more || (p.total >= 0 && listed != p.total) {
		counts = append(counts, fmt.Sprintf("已列出 %d 条", listed))
	}
	note := strings.Join(counts, "，")
	if n := len(p.unlisted); n > 0 {
		note += fmt.Sprintf("（%d 条缺少媒体信息）", n)
	}
	if note == "" {
		return ""
	}
	return note + "。"
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
	return Target{Media: ps.Media, Season: ps.Season, StartEpisode: ps.StartEpisode, EpisodeGroup: ps.EpisodeGroup}
}

// pickPast asks before subscribing past index again, unless it is already
// subscribed; that check is a courtesy, so a failed one still asks.
func (e *Engine) pickPast(ctx context.Context, sess session, index int) Reply {
	if index < 0 || index >= len(sess.past) {
		return Reply{Notice: msgInvalidChoice}
	}
	ps := sess.past[index]
	target := pastTarget(ps)
	sub, err := e.backend.FindSubscription(ctx, target)
	if err != nil {
		e.log.Warn("subscription lookup failed", "media", ps.Media.ID, "err", err)
	}
	if err == nil && sub.ID != 0 {
		return Reply{Notice: msgAlreadyAgain}
	}
	sess.target = &target
	e.store.put(sess)
	question := Line(Strong(fmt.Sprintf("要重新订阅%s吗？", targetName(target))))
	rows := [][]Button{{{Label: msgResubscribeLabel, Data: data(sess.id, actionResubscribe, index), Style: StylePrimary}}, {cancelButton(sess.id)}}
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
	_, err := e.written(target, func() (int, error) { return e.backend.Resubscribe(ctx, ps) })
	rows := [][]Button{append([]Button{backTo(sess.id, actionHistoryKind, int(sess.kind))}, browseRow(sess.id)...)}
	if err != nil {
		return card{Media: ps.Media}.replyLines(e.failure("resubscribe", err).Text, rows)
	}
	done := fmt.Sprintf("✅ 已经帮你重新订阅%s啦%s ヾ(≧▽≦*)o", targetName(target), noticeEnding)
	return card{Media: ps.Media}.reply(Line(Plain(done)), rows)
}

// samePick reports whether the confirmable target is past, so a button left
// from an earlier pick cannot subscribe something else.
func samePick(t Target, ps PastSubscription) bool {
	return t.Media.Source == ps.Media.Source && t.Media.ID == ps.Media.ID && seasonOf(t.Season) == seasonOf(ps.Season) &&
		t.EpisodeGroup == ps.EpisodeGroup
}

// noSeason stands for a movie's missing season, unlike any season number.
const noSeason = -1

func seasonOf(season *int) int {
	if season == nil {
		return noSeason
	}
	return *season
}
