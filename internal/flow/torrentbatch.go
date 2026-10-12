package flow

import (
	"context"
	"fmt"
	"slices"
)

const (
	actionTorrentMulti    = "tm" // ticks releases on the list instead of opening them
	actionTorrentTick     = "tk" // arg: index into session torrents to tick or untick
	actionTorrentBatch    = "tb" // asks to download every ticked release
	actionTorrentBatchGet = "tz" // downloads every ticked release

	// maxTickedReleases bounds the downloads one confirmation adds.
	maxTickedReleases = 20

	msgTickReleases   = "多选…"
	msgTickedTag      = "已选"
	msgTickOne        = "先勾选至少一个资源嘛～"
	msgTickedTooMany  = "一次最多选 %d 个哦～"
	msgBatchAsk       = "要下载这 %d 个资源吗？"
	msgBatchHitAndRun = "⚠️ 其中有 H&R 资源，下完要保种够时间，不然站点会记过哦。"
	msgBatchUnknown   = "标着「没能确认」的资源不知道有没有开始下载…先用 /tasks 看一眼，确认没在下再来找我哦。"
)

// batchHead and batchResultHead name the columns of the ticked releases
// and of how downloading each went.
var (
	batchHead       = []string{"资源", "集", "站点", "大小"}
	batchResultHead = []string{"资源", "结果"}
)

// chooseTorrentBatch applies the steps that download several releases at
// once; ok is false for other actions.
func (e *Engine) chooseTorrentBatch(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionTorrentMulti:
		if sess.streaming && (!sess.searchDone || !sess.searchFinal) {
			return Reply{Notice: msgTorrentPartial}, true
		}
		sess.ticking = true
		return e.releases(sess), true
	case actionTorrentTick:
		return e.tickRelease(sess, p.arg), true
	case actionTorrentBatch:
		return e.askBatch(sess), true
	}
	return Reply{}, false
}

// tickRelease ticks or unticks release index and shows the list page it
// is on.
func (e *Engine) tickRelease(sess session, index int) Reply {
	if !sess.ticking || index < 0 || index >= len(sess.torrents) || sess.focus == nil {
		return Reply{Notice: msgInvalidChoice}
	}
	if !slices.Contains(sess.ticked, index) && len(sess.ticked) >= maxTickedReleases {
		return Reply{Notice: fmt.Sprintf(msgTickedTooMany, maxTickedReleases)}
	}
	sess.ticked = toggled(sess.ticked, index)
	return e.listPageWith(sess, torrentList(sess, *sess.focus), data(sess.id, actionTorrentTick, index))
}

// askBatch shows the ticked releases under the card and asks to download
// them all.
func (e *Engine) askBatch(sess session) Reply {
	if !sess.ticking || sess.focus == nil {
		return Reply{Notice: msgInvalidChoice}
	}
	if len(sess.ticked) == 0 {
		return Reply{Notice: msgTickOne}
	}
	sess.batch = slices.Clone(sess.ticked)
	sess.plan = nil
	sess.searchView = torrentBatchView
	e.store.put(sess)
	rows := make([][]Span, 0, len(sess.batch))
	var total float64
	hitAndRun := false
	for _, i := range sess.batch {
		t := sess.torrents[i]
		rows = append(rows, []Span{Strong(truncate(t.Title, listTitleRunes)), Plain(releaseEpisodes(t)), Plain(siteOf(t)), Plain(size(t.Size))})
		total += t.Size
		hitAndRun = hitAndRun || t.HitAndRun
	}
	text := Lines(Table(batchHead, rows...), Line(Plain(fmt.Sprintf("一共 %s", size(total)))))
	if hitAndRun {
		text = append(text, Callout(Plain(msgBatchHitAndRun)))
	}
	text = append(text, Line(Strong(fmt.Sprintf(msgBatchAsk, len(sess.batch)))))
	buttons := [][]Button{{{Label: msgDownloadLabel, Data: data(sess.id, actionTorrentBatchGet, 0)}}, {cancelButton(sess.id)}}
	return sess.picked.replyLines(text, buttons)
}

// batchOutcome is what happened to one release of a batch.
type batchOutcome struct {
	release Torrent
	state   submissionState
	err     error // set when MoviePilot refused or the result is unknown
	unknown bool
}

// downloadBatch consumes confirmation before starting one serial API worker.
// Polling only reads its progress; it never submits or retries a resource.
func (e *Engine) downloadBatch(_ context.Context, sess session) Reply {
	if len(sess.batch) == 0 || sess.focus == nil || sess.submission != nil {
		return Reply{Notice: msgInvalidChoice}
	}
	var releases []Torrent
	for _, i := range sess.batch {
		if sess.streaming && !e.currentTorrent(sess, i) {
			return Reply{Notice: msgTorrentChanged}
		}
		releases = append(releases, sess.torrents[i])
	}
	sess.hunt.stop()
	sess.hunt, sess.searchView = nil, ""
	b := newDownloadBatch(int(sess.ticket), releases)
	sess.submission = b
	sess.batch, sess.ticked, sess.target, sess.release = nil, nil, nil, nil
	sess.plan = nil
	sess.torrents = nil
	e.store.put(sess)
	reply := submissionReply(sess, b.read())
	e.work.Go(func() { e.submitBatch(b, releases) })
	return reply
}

// downloadOne adds one release of a batch.
func (e *Engine) downloadOne(ctx context.Context, t Torrent) batchOutcome {
	out := batchOutcome{release: Torrent{Title: t.Title}, state: submissionDone}
	id, err := e.backend.Download(ctx, t)
	if err == nil && id == "" {
		err = fmt.Errorf("download answered no id")
	}
	if err != nil {
		_, safe := UserMessage(err)
		out.err, out.unknown = err, !safe
		if out.unknown {
			e.log.Error("download result unknown", "media", t.Media.ID, "id", id, "err", err)
		}
		return out
	}
	return out
}

// batchLines tables each release's outcome, e.g. Dune.2021… | ✅ 开始下载.
func (e *Engine) batchLines(target Target, outcomes []batchOutcome) Text {
	rows := make([][]Span, 0, len(outcomes))
	started, unknown := 0, false
	for _, o := range outcomes {
		unknown = unknown || o.unknown
		if o.state == submissionDone && o.err == nil {
			started++
		}
		rows = append(rows, []Span{Strong(truncate(o.release.Title, listTitleRunes)), Plain(o.label())})
	}
	target.StartEpisode = 0
	head := fmt.Sprintf("%s的下载结果来啦：", targetName(target))
	text := Lines(Line(Strong(head)), Table(batchResultHead, rows...))
	if started > 0 {
		done := fmt.Sprintf("✅ 开始下载 %d 个资源啦%s ヾ(≧▽≦*)o", started, noticeEnding)
		text = append(text, Line(Plain(done)))
	}
	if unknown {
		text = append(text, Line(Plain(msgBatchUnknown)))
	}
	return text
}
