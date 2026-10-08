package e2e

import (
	"cmp"
	"slices"
	"strconv"
	"time"
)

// numberTransfer fills the history id and, unless the scenario supplies
// one, a stable completion date. Retries may later change only that date.
func numberTransfer(file transfer, id int) transfer {
	file.ID = id
	if file.Date == "" {
		file.Date = epoch.Add(time.Duration(id) * time.Second).Format(time.DateTime)
	}
	return file
}

// transferHistory mirrors MoviePilot's status filter and date DESC order.
// The API promises no tie-break: descending ids put an old retried row on
// a later page even when it finished in the same second as newer rows.
func transferHistory(records []transfer, status string) []transfer {
	out := slices.Clone(records)
	if wanted, err := strconv.ParseBool(status); err == nil {
		out = slices.DeleteFunc(out, func(file transfer) bool { return file.Status != wanted })
	}
	slices.SortFunc(out, func(a, b transfer) int {
		return cmp.Or(cmp.Compare(b.Date, a.Date), cmp.Compare(b.ID, a.ID))
	})
	return out
}

// failedTransfer records a failed attempt, invisible to status=true.
func (f *fakeMoviePilot) failedTransfer(file transfer) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	file = numberTransfer(file, len(f.transfers)+1)
	file.Status = false
	f.transfers = append([]transfer{file}, f.transfers...)
	return file.ID
}

// retryTransfer mirrors v3.1.0's history upsert: a successful retry changes
// status and completion date in place without allocating a new row id.
func (f *fakeMoviePilot) retryTransfer(id int, date string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index := slices.IndexFunc(f.transfers, func(file transfer) bool { return file.ID == id })
	if index < 0 {
		f.t.Fatalf("missing transfer %d", id)
	}
	f.transfers[index].Status, f.transfers[index].Date = true, date
}
