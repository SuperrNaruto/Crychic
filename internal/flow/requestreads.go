package flow

import (
	"context"
	"errors"
	"slices"
	"sync"
)

type requestSources struct {
	subs       []Subscription
	tasks      taskReads
	failures   FailedTransfers
	subErr     error
	failureErr error
}

type requestDetail struct {
	record            RequestRecord
	progress          subscriptionDetail
	subscriptionFound bool
	subErr            error
	failures          []TransferFailure
	failureErr        error
	limited           bool
	ambiguousDownload bool
	unknownScope      bool
	unscopedFailures  bool
	unmapped          bool
}

var (
	errRequestMapping  = errors.New("request episode group mapping is unknown")
	errRequestInstance = errors.New("request download instance is unknown")
)

// Read shared lists once per explicit refresh. Library queries belong to
// individual details, not an unbounded fan-out across all owned requests.
func (e *Engine) readRequestSources(ctx context.Context, records []RequestRecord) requestSources {
	ctx, cancel := context.WithTimeout(ctx, cosmeticTimeout)
	defer cancel()
	var read requestSources
	var wg sync.WaitGroup
	if slices.ContainsFunc(records, func(r RequestRecord) bool { return r.SubscriptionID > 0 }) {
		wg.Go(func() { read.subs, read.subErr = e.backend.Subscriptions(ctx) })
	}
	wg.Go(func() { read.tasks.downloads, read.tasks.downloadErr = e.backend.Downloads(ctx) })
	wg.Go(func() { read.tasks.jobs, read.tasks.jobErr = e.backend.Transfers(ctx) })
	wg.Go(func() { read.failures, read.failureErr = e.backend.FailedTransfers(ctx) })
	wg.Wait()
	if read.tasks.downloadErr != nil {
		read.tasks.downloads = nil
	}
	if read.tasks.jobErr != nil {
		read.tasks.jobs = nil
	}
	return read
}

func (r RequestRecord) subscription() Subscription {
	m := r.Target.Media
	return Subscription{
		ID: r.SubscriptionID, Source: m.Source, MediaID: m.ID, Title: m.Title, Year: m.Year,
		Kind: m.Kind, Season: r.Target.Season, StartEpisode: r.Target.StartEpisode,
		Total: r.Total, BestVersion: r.Target.BestVersion,
	}
}

func (r RequestRecord) matchesSubscription(sub Subscription) bool {
	base := subscriptionDetail{sub: r.subscription()}
	return r.SubscriptionID > 0 && r.SubscriptionID == sub.ID &&
		base.sameMedia(sub.Kind, sub.Source, sub.MediaID) && base.sameSeason(sub.Season)
}

func (read requestSources) observe(record RequestRecord) requestDetail {
	d := requestDetail{record: record, failureErr: read.failureErr, limited: read.failures.Limited, unmapped: record.Unmapped}
	if record.SubscriptionID > 0 {
		d.subErr = read.subErr
	}
	d.progress = subscriptionDetail{
		sub: record.subscription(), downloadsErr: read.tasks.downloadErr, transfersErr: read.tasks.jobErr,
	}
	if i := slices.IndexFunc(read.subs, record.matchesSubscription); i >= 0 && read.subErr == nil {
		d.progress.sub, d.subscriptionFound = read.subs[i], true
		d.unmapped = read.subs[i].EpisodeGroup != ""
	}
	if d.unmapped {
		d.progress.libraryErr, d.progress.downloadsErr, d.progress.transfersErr = errRequestMapping, errRequestMapping, errRequestMapping
		return d
	}
	d.progress.downloads = d.downloads(read.tasks.downloads)
	if record.IsDownload && record.ambiguousInstance(read.tasks) {
		d.ambiguousDownload, d.progress.downloads = true, nil
		d.progress.downloadsErr, d.progress.transfersErr = errRequestInstance, errRequestInstance
	}
	d.progress.transfers = d.transfers(read.tasks.jobs)
	for _, failure := range read.failures.Items {
		if d.matchesFailure(failure) {
			if record.IsDownload {
				d.unscopedFailures = true
			} else {
				d.failures = append(d.failures, failure)
			}
		}
	}
	return d
}

func (d *requestDetail) downloads(all []Download) []Download {
	var out []Download
	for _, download := range all {
		if !d.progress.sameMedia(download.Kind, download.Source, download.MediaID) {
			continue
		}
		if d.record.IsDownload && download.ID != d.record.Download {
			continue
		}
		if !d.knownSeason(download.Season) {
			continue
		}
		d.unknownScope = d.unknownScope || (d.progress.sub.Kind == TV && len(download.Episodes) == 0)
		if d.includes(download.Episodes) {
			out = append(out, download)
		}
	}
	return out
}

func (d *requestDetail) transfers(all []TransferJob) []TransferJob {
	if d.ambiguousDownload {
		return nil
	}
	var out []TransferJob
	for _, job := range all {
		if !d.progress.sameMedia(job.Kind, job.Source, job.MediaID) || !d.knownSeason(job.Season) {
			continue
		}
		job.Files = slices.DeleteFunc(slices.Clone(job.Files), func(f TransferFile) bool {
			return (d.record.IsDownload && f.Download != d.record.Download) || !d.includes(f.Episodes)
		})
		if d.progress.sub.Kind == TV && slices.ContainsFunc(job.Files, func(f TransferFile) bool { return len(f.Episodes) == 0 }) {
			d.unknownScope = true
		}
		if len(job.Files) > 0 {
			out = append(out, job)
		}
	}
	return out
}

func (d requestDetail) includes(episodes []int) bool {
	if d.record.IsDownload || d.progress.sub.Kind == Movie || len(episodes) == 0 {
		return true
	}
	s := d.progress.sub
	return slices.ContainsFunc(episodes, func(ep int) bool {
		return ep >= max(s.StartEpisode, 1) && (s.Total <= 0 || ep <= s.Total)
	})
}

func (d requestDetail) matchesFailure(f TransferFailure) bool {
	if !d.progress.sameMedia(f.Kind, f.Source, f.MediaID) || !d.progress.sameSeason(f.Season) || f.EpisodeGroup != "" {
		return false
	}
	if d.record.IsDownload {
		return f.Download == d.record.Download
	}
	return (f.Kind == Movie || len(f.Episodes) > 0) && d.includes(f.Episodes)
}

func (e *Engine) readRequestDetail(ctx context.Context, record RequestRecord) requestDetail {
	ctx, cancel := context.WithTimeout(ctx, cosmeticTimeout)
	defer cancel()
	d := requestDetail{record: record, progress: subscriptionDetail{sub: record.subscription()}}
	var read requestSources
	var library Library
	var libraryErr error
	var wg sync.WaitGroup
	if record.CompletedAt.IsZero() {
		wg.Go(func() { read = e.readRequestSources(ctx, []RequestRecord{record}) })
	}
	if !record.Unmapped {
		wg.Go(func() { library, libraryErr = e.backend.Library(ctx, record.Target.Media) })
	}
	wg.Wait()
	if record.CompletedAt.IsZero() {
		d = read.observe(record)
	}
	if d.unmapped {
		return d
	}
	if record.Unmapped {
		d.progress.libraryErr = errRequestMapping
		return d
	}
	d.progress.library, d.progress.libraryErr = library, libraryErr
	return d
}

func (d *requestDetail) knownSeason(season *int) bool {
	if d.progress.sameSeason(season) {
		return true
	}
	d.unknownScope = d.unknownScope || season == nil
	return false
}
