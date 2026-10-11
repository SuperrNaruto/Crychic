package moviepilot

import "github.com/SuperrNaruto/Crychic/internal/flow"

type subscriptionBatchRecord struct {
	ID         string `json:"batch_id"`
	Source     string `json:"source"`
	State      string `json:"state"`
	Phase      string `json:"phase"`
	Created    string `json:"created_at"`
	Updated    string `json:"updated_at"`
	Total      *int   `json:"total_count"`
	Processed  *int   `json:"processed_count"`
	Finished   *int   `json:"finished_count"`
	Failed     *int   `json:"failed_count"`
	Cancelled  *int   `json:"cancelled_count"`
	Skipped    *int   `json:"skipped_count"`
	CurrentSub *int   `json:"current_subscription_id"`
	CanCancel  bool   `json:"can_cancel"`
	Error      string `json:"error"`
}

func (r subscriptionBatchRecord) validCounts() bool {
	for _, count := range []*int{r.Total, r.Processed, r.Finished, r.Failed, r.Cancelled} {
		if count == nil || *count < 0 {
			return false
		}
	}
	if *r.Total == 0 || *r.Processed > *r.Total {
		return false
	}
	remaining := *r.Processed
	for _, count := range []int{*r.Finished, *r.Failed, *r.Cancelled, deref(r.Skipped)} {
		if count < 0 || count > remaining {
			return false
		}
		remaining -= count
	}
	return remaining == 0
}

func (r subscriptionBatchRecord) batch() flow.SubscriptionSearchBatch {
	return flow.SubscriptionSearchBatch{
		ID: r.ID, Source: r.Source, State: r.State, Phase: r.Phase,
		Created: subscriptionTime(r.Created), Updated: subscriptionTime(r.Updated),
		Total: *r.Total, Processed: *r.Processed, Finished: *r.Finished,
		Failed: *r.Failed, Cancelled: *r.Cancelled, Skipped: deref(r.Skipped),
		CurrentSubscription: deref(r.CurrentSub), CanCancel: r.CanCancel, HasError: r.Error != "",
	}
}

func (r subscriptionBatchRecord) consistentState() bool {
	switch r.State {
	case "completed", "failed", "cancelled", "skipped":
		return *r.Processed == *r.Total
	case "queued", "running", "cancelling":
		return *r.Processed < *r.Total
	}
	// Unknown future states may be displayed as unknown, never controlled.
	return true
}
