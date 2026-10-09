package flow

import (
	"context"
	"sync"
)

// A partial response is usable without pretending the unavailable source has
// no tasks. IDs and actions are derived only from successfully read records.
type taskReads struct {
	downloads   []Download
	jobs        []TransferJob
	downloadErr error
	jobErr      error
}

func (e *Engine) taskLists(ctx context.Context) taskReads {
	var read taskReads
	var wg sync.WaitGroup
	wg.Go(func() { read.downloads, read.downloadErr = e.backend.Downloads(ctx) })
	wg.Go(func() { read.jobs, read.jobErr = e.backend.Transfers(ctx) })
	wg.Wait()
	for _, err := range []error{read.downloadErr, read.jobErr} {
		if err != nil {
			e.log.Warn("task list source unavailable", "err", err)
		}
	}
	if read.downloadErr != nil {
		read.downloads = nil
	}
	if read.jobErr != nil {
		read.jobs = nil
	}
	return read
}

func (r taskReads) empty() bool {
	return r.downloadErr == nil && r.jobErr == nil && len(r.downloads)+len(r.jobs) == 0
}

func (r taskReads) view(id uint64) listView {
	view := taskList(id, r.downloads, r.jobs)
	if r.downloadErr == nil && r.jobErr == nil {
		return view
	}
	if r.downloadErr != nil {
		view.note = "下载状态暂时无法确认"
	}
	if r.jobErr != nil {
		view.note = joinNonEmpty("；", view.note, "整理状态暂时无法确认")
	}
	for _, err := range []error{r.downloadErr, r.jobErr} {
		if msg, safe := UserMessage(err); safe {
			view.note = joinNonEmpty("；", view.note, msg)
			break
		}
	}
	view.menu = [][]Button{{{Label: "刷新", Data: data(id, actionList, 0)}}}
	return view
}
