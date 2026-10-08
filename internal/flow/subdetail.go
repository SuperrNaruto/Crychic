package flow

import (
	"context"
	"errors"
	"slices"
	"sync"
)

const (
	actionSubDetail        = "sd" // arg: subscription id; shows its details
	actionRefreshSubDetail = "su" // arg: subscription id; reads its details afresh
	msgSubDetailTitle      = "📚 订阅详情"
	msgSubDetailFailed     = "暂时查不到订阅详情，稍后再点一次试试吧～"
	msgSubDetailGone       = "这条订阅已经结束或被取消啦，可以回订阅列表或查看订阅历史哦～"
)

// subscriptionDetail keeps successful reads separate from unavailable ones.
// File records are deliberately not fetched: they can reveal private paths,
// and a recorded download/transfer does not establish media-server visibility.
type subscriptionDetail struct {
	sub          Subscription
	library      Library
	downloads    []Download
	transfers    []TransferJob
	upkeep       SubscriptionUpkeep
	libraryErr   error
	downloadsErr error
	transfersErr error
	upkeepErr    error
}

func (e *Engine) chooseSubDetail(ctx context.Context, sess session, p press) (Reply, bool) {
	if p.action != actionSubDetail && p.action != actionRefreshSubDetail {
		return Reply{}, false
	}
	index := sess.subAt(p.arg)
	if index < 0 {
		return Reply{Notice: msgInvalidChoice}, true
	}
	return e.subDetail(ctx, sess, index), true
}

func (e *Engine) subDetail(ctx context.Context, sess session, index int) Reply {
	// Only the list endpoint attaches current execution status. Re-read it
	// on open/refresh, keeping the original list snapshot intact for 返回.
	subs, err := e.backend.Subscriptions(ctx)
	if err != nil {
		e.log.Warn("subscription details unavailable", "err", err)
		if message, safe := UserMessage(err); safe {
			return Reply{Notice: message}
		}
		return Reply{Notice: msgSubDetailFailed}
	}
	i := slices.IndexFunc(subs, sess.subs[index].same)
	if i < 0 {
		e.store.put(sess)
		return subDetailGone(sess, sess.subs[index].Kind)
	}
	detail := e.readSubDetail(ctx, subs[i])
	view := detail.view()
	refresh := Button{Label: "刷新", Data: data(sess.id, actionRefreshSubDetail, sess.subs[index].ID)}
	view.menu = [][]Button{append([]Button{refresh}, pauseButton(sess, subs[i])...)}
	view.footer = browseRow(sess.id)
	return e.listPages(sess, view)
}

// subDetailGone says the subscription ended and offers the history of its
// kind, which a list opened from the calendar never chose.
func subDetailGone(sess session, kind Kind) Reply {
	return Reply{
		Text: Lines(Heading(Plain(msgSubDetailTitle)), Line(Plain(msgSubDetailGone))),
		Buttons: [][]Button{
			{{Label: "订阅历史", Data: data(sess.id, actionHistory, int(kind))}},
			append([]Button{backTo(sess.id, actionSubs, 0)}, browseRow(sess.id)...),
		},
	}
}

func (e *Engine) readSubDetail(ctx context.Context, sub Subscription) subscriptionDetail {
	d := subscriptionDetail{sub: sub}
	ctx, cancel := context.WithTimeout(ctx, cosmeticTimeout)
	defer cancel()
	var wg sync.WaitGroup
	wg.Go(func() { d.upkeep, d.upkeepErr = e.backend.SubscriptionUpkeep(ctx) })
	if sub.Source == "" || sub.MediaID == "" || (sub.Kind == TV && sub.Season == nil) {
		err := errors.New("subscription has no usable media identity or season")
		d.libraryErr, d.downloadsErr, d.transfersErr = err, err, err
	} else {
		media := Media{Source: sub.Source, ID: sub.MediaID, Title: sub.Title, Year: sub.Year, Kind: sub.Kind}
		wg.Go(func() { d.library, d.libraryErr = e.backend.Library(ctx, media) })
		wg.Go(func() { d.downloads, d.downloadsErr = e.backend.Downloads(ctx) })
		wg.Go(func() { d.transfers, d.transfersErr = e.backend.Transfers(ctx) })
	}
	wg.Wait()
	for _, err := range []error{d.libraryErr, d.downloadsErr, d.transfersErr, d.upkeepErr} {
		if err != nil {
			e.log.Warn("subscription progress unavailable", "subscription", sub.ID, "err", err)
		}
	}
	return d
}
