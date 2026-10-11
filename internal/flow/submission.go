package flow

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

const (
	actionBatchProgress = "br"
	actionBatchStop     = "bs"
	msgBatchStop        = "停止后续提交"
	msgBatchStopNote    = "当前一项会等到结果或超时；已创建的下载会保留，不会暂停或删除哦～"
)

type submissionState int

const (
	submissionQueued submissionState = iota
	submissionRunning
	submissionDone
	submissionSkipped
)

// downloadBatch tracks API admission, not downloader scheduling or progress.
// Progress snapshots hold only titles/results, never secret-bearing releases.
type downloadBatch struct {
	mu        sync.Mutex
	token     int
	items     []batchOutcome
	processed int
	stopping  bool
	finished  time.Time
}

type submissionProgress struct {
	items     []batchOutcome
	processed int
	stopping  bool
	done      bool
}

func newDownloadBatch(token int, releases []Torrent) *downloadBatch {
	b := &downloadBatch{token: token}
	for _, t := range releases {
		b.items = append(b.items, batchOutcome{release: Torrent{Title: t.Title}})
	}
	b.items[0].state = submissionRunning
	return b
}

func (b *downloadBatch) read() submissionProgress {
	b.mu.Lock()
	defer b.mu.Unlock()
	return submissionProgress{slices.Clone(b.items), b.processed, b.stopping, !b.finished.IsZero()}
}

// Admission and stop share a lock. Once admitted, the current call runs to its
// result or backend timeout; stopping never cancels an in-flight write.
func (b *downloadBatch) admit(ctx context.Context) (int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopping || ctx.Err() != nil || b.processed == len(b.items) {
		return 0, false
	}
	b.items[b.processed].state = submissionRunning
	return b.processed, true
}

func (b *downloadBatch) accept(index int, result batchOutcome) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items[index] = result
	b.processed++
}

func (b *downloadBatch) stop() {
	b.mu.Lock()
	b.stopping = true
	b.mu.Unlock()
}

func (b *downloadBatch) finish(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := b.processed; i < len(b.items); i++ {
		b.items[i].state = submissionSkipped
	}
	b.finished = now
}

func (b *downloadBatch) retained(now time.Time, ttl time.Duration) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.finished.IsZero() || now.Before(b.finished.Add(ttl))
}

func (e *Engine) submitBatch(b *downloadBatch, releases []Torrent) {
	defer func() { b.finish(e.now()) }()
	for {
		i, ok := b.admit(e.runCtx)
		if !ok {
			return
		}
		b.accept(i, e.downloadOne(e.runCtx, releases[i]))
		releases[i] = Torrent{}
	}
}

func (e *Engine) batchProgress(sess session, p press) Reply {
	b := sess.submission
	if b == nil || p.arg != b.token {
		return Reply{Notice: msgInvalidChoice}
	}
	if p.action == actionBatchStop {
		b.stop()
	}
	progress := b.read()
	if !progress.done {
		e.store.put(sess)
		return submissionReply(sess, progress)
	}
	sess.submission = nil
	e.store.put(sess)
	text := e.batchLines(*sess.focus, progress.items)
	if progress.processed < len(progress.items) {
		text = append(text, Line(Plain("已停止后续提交；已创建的下载保持不变哦～")))
	}
	return e.navigate(sess, actionTorrentBatchGet, sess.picked.replyLines(text, nil))
}

func submissionReply(sess session, p submissionProgress) Reply {
	status := "正在提交"
	if p.stopping {
		status = "正在停止后续提交"
	}
	text := Lines(Line(Strong(fmt.Sprintf("%s %d/%d", status, p.processed, len(p.items)))))
	var rows [][]Span
	for _, item := range p.items {
		rows = append(rows, []Span{Strong(truncate(item.release.Title, listTitleRunes)), Plain(item.label())})
	}
	text = append(text, Table(batchResultHead, rows...), Remark(msgBatchStopNote))
	var buttons [][]Button
	if !p.stopping {
		buttons = [][]Button{{{Label: msgBatchStop, Data: data(sess.id, actionBatchStop, sess.submission.token)}}}
	}
	reply := sess.picked.replyLines(text, buttons)
	reply.Follow = data(sess.id, actionBatchProgress, sess.submission.token)
	return reply
}

func (o batchOutcome) label() string {
	switch o.state {
	case submissionQueued:
		return "等待提交"
	case submissionRunning:
		return "提交中"
	case submissionSkipped:
		return "未提交"
	}
	if o.unknown {
		return "⚠️ 没能确认"
	}
	if o.err != nil {
		return "⚠️ " + failureText(o.err)
	}
	return "✅ 开始下载"
}
