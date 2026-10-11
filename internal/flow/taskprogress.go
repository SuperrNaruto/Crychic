package flow

import (
	"context"
	"slices"
)

// withProgress gives j's running files the byte progress of the one active
// record naming the same media, season, episodes and download. A record
// that more than one running file could own, or no record at all, leaves
// files with their states only, so progress of another file or of the
// whole queue never shows as this task's.
func (e *Engine) withProgress(ctx context.Context, j TransferJob) TransferJob {
	if !slices.ContainsFunc(j.Files, running) {
		return j
	}
	records, err := e.backend.TransferProgress(ctx)
	if err != nil {
		e.log.Debug("transfer progress unknown", "err", err)
		return j
	}
	files := slices.Clone(j.Files)
	for i, f := range files {
		if !running(f) || countFunc(j.Files, func(other TransferFile) bool { return running(other) && sameFile(f, other) }) != 1 {
			continue
		}
		owned := func(r FileProgress) bool { return ownsProgress(j, f, r) }
		if countFunc(records, owned) != 1 {
			continue
		}
		if r := records[slices.IndexFunc(records, owned)]; r.Active {
			files[i].Percent = &r.Percent
		}
	}
	j.Files = files
	return j
}

func running(f TransferFile) bool { return f.State == FileRunning }

// sameFile reports whether two queue files carry the same identity.
func sameFile(a, b TransferFile) bool {
	return slices.Equal(a.Episodes, b.Episodes) && a.Download == b.Download && a.Downloader == b.Downloader
}

// ownsProgress reports whether record r names file f of job j.
func ownsProgress(j TransferJob, f TransferFile, r FileProgress) bool {
	return r.Source == j.Source && r.MediaID == j.MediaID && seasonOf(r.Season) == seasonOf(j.Season) &&
		sameFile(f, TransferFile{Episodes: r.Episodes, Download: r.Download, Downloader: r.Downloader})
}

func countFunc[T any](items []T, match func(T) bool) int {
	n := 0
	for _, item := range items {
		if match(item) {
			n++
		}
	}
	return n
}
