package flow

import "time"

// SubscriptionSearchReceipt distinguishes a newly enqueued single batch
// from coalescing an existing batch. The latter never grants control.
type SubscriptionSearchReceipt struct {
	BatchID string
	Ongoing bool
}

// SubscriptionSearchBatch contains only safe execution facts, not resource
// records, site names, error messages or evidence about library visibility.
type SubscriptionSearchBatch struct {
	ID                  string
	Source              string
	State               string
	Phase               string
	Created             time.Time
	Updated             time.Time
	Total               int
	Processed           int
	Finished            int
	Failed              int
	Cancelled           int
	Skipped             int
	CurrentSubscription int
	CanCancel           bool
	HasError            bool
}

const (
	searchAccepted = "accepted"
	searchOngoing  = "ongoing"
	searchRefused  = "refused"
	searchUnknown  = "unknown"

	subSearchPromptPage = "prompt"
	subSearchStatusPage = "status"
	subSearchStopPage   = "stop"
)

// The latest attempt is session-owned, immutable between transitions and
// never restored from navigation or inferred from MoviePilot's batch list.
type subscriptionSearch struct {
	sequence int
	base     Subscription
	batchID  string
	created  time.Time
	outcome  string
	message  string
	last     *SubscriptionSearchBatch
	fresh    bool
	problem  string
	on       bool
	until    time.Time
	stop     string // nonempty consumes cancellation, including unknown results
	stopNote string
}

func (batch SubscriptionSearchBatch) terminal() bool {
	switch batch.State {
	case "completed", "failed", "cancelled", "skipped":
		return true
	}
	return false
}

func (batch SubscriptionSearchBatch) active() bool {
	return batch.State == "queued" || batch.State == "running" || batch.State == "cancelling"
}

func (run subscriptionSearch) matches(batch SubscriptionSearchBatch) bool {
	if batch.ID != run.batchID || batch.Source != "manual" || batch.Total != 1 || batch.Created.IsZero() {
		return false
	}
	if !run.created.IsZero() && !run.created.Equal(batch.Created) {
		return false
	}
	if batch.CurrentSubscription != 0 && batch.CurrentSubscription != run.base.ID {
		return false
	}
	return !batch.active() || batch.CurrentSubscription == run.base.ID
}

func (run subscriptionSearch) canStop() bool {
	if run.last == nil || !run.fresh || run.stop != "" {
		return false
	}
	batch := run.last
	return batch.CanCancel && batch.Processed == 0 &&
		(batch.State == "queued" || batch.State == "running") && subExecutionText[batch.Phase] != ""
}

func (run subscriptionSearch) canSearchAgain() bool {
	return run.outcome == searchRefused || (run.fresh && run.last != nil && run.last.terminal())
}

func copySubSearch(sess session) session {
	run := *sess.subSearch
	sess.subSearch = &run
	return sess
}
