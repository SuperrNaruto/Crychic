package flow

import "time"

// RequestRecord is a safe, read-only projection of a locally owned watch
// or a recent completion. Key is internal; it never appears in chat.
type RequestRecord struct {
	Key            string
	SubscriptionID int
	Download       string
	IsDownload     bool
	Target         Target
	Total          int
	Delivered      []int
	MovieDelivered bool
	Unmapped       bool // a known custom episode group cannot use default season numbering
	CompletedAt    time.Time
}

// FailedTransferLimit bounds one diagnostic refresh, not arrival polling.
const FailedTransferLimit = 100

type TransferFailureStage int

const (
	FailureUnknown TransferFailureStage = iota
	FailureSource
	FailureRecognition
	FailurePlanning
	FailureDestination
	FailureTransfer
	FailureOverwrite
	FailureCleanup
	FailureExecution
)

// TransferFailure deliberately excludes paths, error text and the upstream
// free-text recovery_action. Unknown stage strings never cross this boundary.
type TransferFailure struct {
	ID             int
	Source         string
	MediaID        string
	Kind           Kind
	Season         *int
	Episodes       []int
	Download       string
	EpisodeGroup   string
	Stage          TransferFailureStage
	RetryExhausted bool
	AutoPaused     bool
}

// FailedTransfers can retain a successfully read page alongside an error.
// Limited means older records were deliberately not fetched.
type FailedTransfers struct {
	Items   []TransferFailure
	Limited bool
}
