package flow

import "slices"

// warmBatch is how many picks a warming round looks up at once.
const warmBatch = 3

// warmWeek fills the note cache for every weekday of a calendar in the
// background, a few picks at a time and beginning with day, so switching
// weekday shows notes at once. Only one warm runs at a time; picks that have
// a fresh note or are being looked up are left alone.
func (e *Engine) warmWeek(picks []Media, day int) {
	if !e.calendarWarm || !e.notes.beginWarm() {
		return
	}
	order := fromDay(picks, day)
	e.work.Go(func() {
		defer e.notes.endWarm()
		for len(order) > 0 {
			stale := e.notes.unclaimed(order, e.now())
			if len(stale) == 0 {
				return
			}
			n := min(warmBatch, len(stale))
			if !e.warmBatchDone(stale[:n]) {
				return
			}
			order = stale[n:]
		}
	})
}

// warmBatchDone looks up batch and waits for it; false once the app is stopping.
func (e *Engine) warmBatchDone(batch []Media) bool {
	done := e.joined(e.startNotes(batch))
	if done == nil {
		return e.runCtx.Err() == nil
	}
	select {
	case <-done:
		return true
	case <-e.runCtx.Done():
		return false
	}
}

// fromDay is picks, sorted by weekday, rotated to begin with day's.
func fromDay(picks []Media, day int) []Media {
	first := slices.IndexFunc(picks, func(m Media) bool { return m.Weekday >= day })
	if first <= 0 {
		return slices.Clone(picks)
	}
	return append(slices.Clone(picks[first:]), picks[:first]...)
}
