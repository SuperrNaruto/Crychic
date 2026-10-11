package flow

import (
	"context"
	"fmt"
	"slices"
	"sync"
)

const (
	actionSeriesPlan = "jq" // ticks the series' missing parts to subscribe
	actionSeriesTick = "jt" // arg: index into the parts; ticks or unticks it
	actionSeriesGet  = "jy" // subscribes every ticked part

	// seriesReads bounds the library reads running at once.
	seriesReads = 4

	labelSeriesPlan = "补齐缺少的作品…"
	msgPickParts    = "想补哪几部呀？可以多选，确认后逐部订阅～"
	msgPickOnePart  = "先挑至少一部嘛～"
	msgPartsUnknown = "部分作品的状态暂时无法确认，点编号可以查看详情～"
)

// partState is where one part of a series stands, from the library,
// downloads and subscriptions read together.
type partState int

const (
	partUnknown partState = iota // a source needed to tell could not be read
	partMissing
	partSubscribed
	partPaused
	partDownloading
	partHeld
)

var partLabels = map[partState]string{
	partUnknown: "状态未知", partMissing: "未获取", partSubscribed: "已订阅",
	partPaused: "已暂停订阅", partDownloading: "下载中", partHeld: "已入库",
}

// seriesFacts are the reads a series' states are decided from.
type seriesFacts struct {
	subs         []Subscription
	subsErr      error
	downloads    []Download
	downloadsErr error
	library      []Library
	libraryErr   []error
}

// seriesStates reads the subscriptions and downloads once and each part's
// library at a few at once, all within the cosmetic budget; a read that
// fails or runs out leaves the parts it decides unknown.
func (e *Engine) seriesStates(ctx context.Context, parts []Media) []partState {
	ctx, cancel := context.WithTimeout(ctx, cosmeticTimeout)
	defer cancel()
	f := seriesFacts{library: make([]Library, len(parts)), libraryErr: make([]error, len(parts))}
	var wg sync.WaitGroup
	wg.Go(func() { f.subs, f.subsErr = e.backend.Subscriptions(ctx) })
	wg.Go(func() { f.downloads, f.downloadsErr = e.backend.Downloads(ctx) })
	slots := make(chan struct{}, seriesReads)
	for i, m := range parts {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			f.library[i], f.libraryErr[i] = e.library(ctx, m)
		})
	}
	wg.Wait()
	states := make([]partState, len(parts))
	for i, m := range parts {
		states[i] = f.state(i, m)
	}
	return states
}

// state decides part i: what the library holds first, then what is on its
// way, then what is subscribed; missing only when every source was read.
func (f seriesFacts) state(i int, m Media) partState {
	if f.libraryErr[i] == nil && f.library[i].Movie {
		return partHeld
	}
	if f.downloadsErr == nil && slices.ContainsFunc(f.downloads, func(d Download) bool {
		return d.Source == m.Source && d.MediaID == m.ID && d.Kind == m.Kind
	}) {
		return partDownloading
	}
	if f.subsErr == nil {
		if i := slices.IndexFunc(f.subs, func(s Subscription) bool {
			return s.Source == m.Source && s.MediaID == m.ID && s.Kind == m.Kind
		}); i >= 0 {
			return map[bool]partState{true: partPaused, false: partSubscribed}[f.subs[i].State == statePaused]
		}
	}
	if f.libraryErr[i] != nil || f.downloadsErr != nil || f.subsErr != nil {
		return partUnknown
	}
	return partMissing
}

// seriesView lists the series' parts with where each stands; a number
// opens a part's card, and the missing ones can be ticked to subscribe.
func seriesView(sess session, heading string) listView {
	view := relatedView(sess, heading)
	for i := range view.entries {
		view.entries[i].text[0].Tag = partLabels[sess.partStates[i]]
	}
	if slices.Contains(sess.partStates, partMissing) {
		view.menu = [][]Button{{{Label: labelSeriesPlan, Data: data(sess.id, actionSeriesPlan, 0)}}}
	}
	if slices.Contains(sess.partStates, partUnknown) {
		view.note = msgPartsUnknown
	}
	return view
}

// chooseSeriesPlan applies the series fill-in actions; ok is false for others.
func (e *Engine) chooseSeriesPlan(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionSeriesPlan:
		return e.partPicker(sess), true
	case actionSeriesTick:
		if p.arg < 0 || p.arg >= len(sess.partStates) || sess.partStates[p.arg] != partMissing {
			return Reply{Notice: msgInvalidChoice}, true
		}
		sess.wanted = toggled(sess.wanted, p.arg)
		return e.partPicker(sess), true
	case actionSeriesGet:
		if len(sess.wanted) == 0 {
			return Reply{Notice: msgPickOnePart}, true
		}
		return e.subscribeParts(ctx, sess), true
	}
	return Reply{}, false
}

// partPicker ticks the missing parts, numbered as in the series list.
func (e *Engine) partPicker(sess session) Reply {
	if !slices.Contains(sess.partStates, partMissing) || len(sess.partStates) != len(sess.results) {
		return Reply{Notice: msgInvalidChoice}
	}
	e.store.put(sess)
	text := Lines(Heading(Plain(fmt.Sprintf("🎬 补齐「%s」的同系列", sess.picked.Media.Title))), Line(Strong(msgPickParts)))
	var ticks []Button
	for i, m := range sess.results {
		if sess.partStates[i] != partMissing {
			continue
		}
		on := slices.Contains(sess.wanted, i)
		spans := []Span{Strong(fmt.Sprintf("%d. %s", i+1, m.Title))}
		if m.Year != "" {
			spans = append(spans, Plain(" · "+m.Year))
		}
		text = append(text, Ticked(on, spans...))
		label := fmt.Sprint(i + 1)
		if on {
			label = "✓ " + label
		}
		ticks = append(ticks, Button{Label: label, Data: data(sess.id, actionSeriesTick, i)})
	}
	get := Button{Label: fmt.Sprintf("订阅所选 %d 部", len(sess.wanted)), Data: data(sess.id, actionSeriesGet, 0)}
	rows := append(grid(ticks, gridColumns), []Button{get}, []Button{cancelButton(sess.id)})
	return Reply{Text: text, Buttons: rows}
}

// partOutcome is what happened to one ticked part.
type partOutcome struct {
	media    Media
	existing bool
	err      error
}

// subscribeParts subscribes each ticked part once, in list order, skipping
// any subscribed meanwhile, and tables every part's outcome. The ticks are
// consumed before the first write.
func (e *Engine) subscribeParts(ctx context.Context, sess session) Reply {
	wanted := sess.wanted
	sess.wanted = nil
	e.settle(sess)
	rows := make([][]Span, 0, len(wanted))
	subscribed := false
	for _, i := range wanted {
		o := e.subscribePart(ctx, sess.results[i])
		result := "✅ 已订阅"
		switch {
		case o.err != nil:
			result = "⚠️ " + failureText(o.err)
		case o.existing:
			result = "ℹ️ 已在订阅中"
		}
		rows = append(rows, []Span{Strong(fmt.Sprintf("《%s》", o.media.Title)), Plain(result)})
		subscribed = subscribed || o.err == nil
	}
	text := Lines(Heading(Plain("🎬 同系列补齐结果")), Table(partOutcomeHead, rows...))
	if subscribed {
		text = append(text, Line(Plain(msgWillNotify)))
	}
	return Reply{Text: text}
}

var partOutcomeHead = []string{"作品", "结果"}

func (e *Engine) subscribePart(ctx context.Context, m Media) partOutcome {
	target := Target{Media: m}
	out := partOutcome{media: m}
	sub, err := e.backend.FindSubscription(ctx, target)
	if err == nil && sub.ID != 0 {
		out.existing = true
		return out
	}
	if err == nil {
		_, err = e.submit(ctx, target)
	}
	if err != nil {
		e.log.Error("series part subscription failed", "media", m.ID, "err", err)
		out.err = err
	}
	return out
}
