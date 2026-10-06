package flow

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

const (
	actionMulti      = "a" // opens the multi-season picker
	actionTick       = "t" // arg: season number to tick or untick
	actionSubscribed = "y" // subscribes every ticked season

	msgPickSeasons = "选择要订阅的季（可多选，每季从第 1 集开始）："
	msgPickOne     = "请先选择至少一季。"
)

// chooseSeasons applies the multi-season actions; ok is false for others.
func (e *Engine) chooseSeasons(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionMulti:
		return e.tickBox(sess), true
	case actionTick:
		if !slices.Contains(sess.selectable(), p.arg) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		sess.chosen = toggled(sess.chosen, p.arg)
		return e.tickBox(sess), true
	case actionSubscribed:
		if len(sess.chosen) == 0 {
			return Reply{Notice: msgPickOne}, true
		}
		return e.subscribeSeasons(ctx, sess), true
	}
	return Reply{}, false
}

// selectable lists the seasons worth subscribing: those not fully held.
func (sess session) selectable() []int {
	var out []int
	for _, s := range sess.seasons {
		if !sess.wholeSeasonHeld(s) {
			out = append(out, s.Number)
		}
	}
	return out
}

// toggled returns chosen with season added or removed, sorted.
func toggled(chosen []int, season int) []int {
	if i := slices.Index(chosen, season); i >= 0 {
		return slices.Delete(slices.Clone(chosen), i, i+1)
	}
	return slices.Sorted(slices.Values(append(slices.Clone(chosen), season)))
}

// tickBox shows the multi-season picker with the current ticks.
func (e *Engine) tickBox(sess session) Reply {
	e.store.put(sess)
	var seasons []Season
	var ticks []Button
	for _, s := range sess.seasons {
		if !slices.Contains(sess.selectable(), s.Number) {
			continue
		}
		label := seasonName(s.Number)
		if slices.Contains(sess.chosen, s.Number) {
			label = "✓ " + label
		}
		seasons = append(seasons, s)
		ticks = append(ticks, Button{Label: label, Data: data(sess.id, actionTick, s.Number)})
	}
	subscribe := Button{Label: fmt.Sprintf("订阅所选 %d 季", len(sess.chosen)), Data: data(sess.id, actionSubscribed, 0)}
	rows := append(grid(ticks, seasonColumns), []Button{subscribe, cancelButton(sess.id)})
	return sess.picked.replyLines(sess.seasonChecklist(msgPickSeasons, seasons, sess.chosen), rows)
}

// seasonOutcome is what happened to one season of a multi-season request.
type seasonOutcome struct {
	season   int
	existing bool
	err      error
	notified bool
}

// subscribeSeasons subscribes each ticked season from its first episode,
// skipping ones already subscribed, and reports every season's outcome.
func (e *Engine) subscribeSeasons(ctx context.Context, sess session) Reply {
	if _, ok := e.store.take(sess.id); !ok {
		return Reply{Text: Sentence(msgExpired)}
	}
	outcomes := make([]seasonOutcome, 0, len(sess.chosen))
	for _, n := range sess.chosen {
		outcomes = append(outcomes, e.subscribeSeason(ctx, sess, n))
	}
	return sess.picked.replyLines(outcomeLines(sess.picked.Media, outcomes), nil)
}

func (e *Engine) subscribeSeason(ctx context.Context, sess session, season int) seasonOutcome {
	target := Target{Media: sess.picked.Media, Season: &season}
	out := seasonOutcome{season: season}
	id, err := e.backend.FindSubscription(ctx, target)
	if err == nil && id != 0 {
		out.existing = true
	}
	if err == nil && id == 0 {
		id, err = e.backend.Subscribe(ctx, target)
	}
	if err != nil {
		e.log.Error("season subscription failed", "season", season, "err", err)
		out.err = err
		return out
	}
	out.notified = e.remember(ctx, sess, subscription{id: id, target: target})
	return out
}

// outcomeLines tables each season's outcome, e.g. 第 2 季 | ✅ 已订阅.
func outcomeLines(m Media, outcomes []seasonOutcome) Text {
	rows := make([][]Span, 0, len(outcomes))
	notified := false
	for _, o := range outcomes {
		result := "✅ 已订阅"
		switch {
		case o.err != nil:
			result = "⚠️ " + failureText(o.err)
		case o.existing:
			result = "ℹ️ 已在订阅中"
		}
		rows = append(rows, []Span{Strong(seasonName(o.season)), Plain(result)})
		notified = notified || o.notified
	}
	text := Lines(Line(Strong(fmt.Sprintf("《%s》订阅结果：", m.Title))), Table(outcomeHead, rows...))
	if notified {
		text = append(text, Line(Plain(msgWillNotify)))
	}
	return text
}

// outcomeHead names the multi-season result's table columns.
var outcomeHead = []string{"季", "结果"}

// failureText is the user-safe reason a call failed.
func failureText(err error) string {
	if msg, ok := UserMessage(err); ok {
		return strings.TrimSuffix(msg, "。")
	}
	return strings.TrimPrefix(strings.TrimSuffix(msgBackendDown, "。"), "⚠️ ")
}
