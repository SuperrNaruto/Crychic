package notify

import "slices"

// TransferCursor follows successful completions, not row creation: a retry
// updates an old id's date. Dates are MoviePilot's local civil timestamps,
// compared as written, never against the bot's clock. IDs deduplicate the
// newest whole second, where a retry can have an id below every previous one.
type TransferCursor struct {
	Date string `json:"date"`
	IDs  []int  `json:"ids,omitempty"`
}

// Advance remembers all successes in the latest second, retaining earlier
// observations of that same second. Neither input is changed.
func (c TransferCursor) Advance(transfers []Transfer) TransferCursor {
	next := TransferCursor{Date: c.Date, IDs: slices.Clone(c.IDs)}
	for _, t := range transfers {
		if t.Date < next.Date {
			continue
		}
		if t.Date > next.Date {
			next.Date, next.IDs = t.Date, nil
		}
		next.IDs = append(next.IDs, t.ID)
	}
	slices.Sort(next.IDs)
	next.IDs = slices.Compact(next.IDs)
	return next
}

func (c TransferCursor) unseen(t Transfer) bool {
	return t.Date > c.Date || (t.Date == c.Date && !slices.Contains(c.IDs, t.ID))
}

// trackTransfers selects arrivals before advancing their checkpoint. Old
// states only knew the largest id: retain their catch-up behavior once,
// without replaying ambiguous older rows, then seed the date cursor from
// the entire read. The caller persists this with pending arrivals and digest.
func (st state) trackTransfers(transfers []Transfer) (state, []Transfer) {
	cursor := TransferCursor{}
	if st.Cursor != nil {
		cursor = *st.Cursor
	}
	var fresh []Transfer
	for _, t := range transfers {
		unseen := cursor.unseen(t)
		if st.Cursor == nil {
			unseen = t.ID > st.LastTransfer
		}
		if unseen {
			fresh = append(fresh, t)
		}
	}
	cursor = cursor.Advance(transfers)
	st.Cursor = &cursor
	return st, fresh
}
