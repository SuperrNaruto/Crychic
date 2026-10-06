// Package notify tells requesters when what they asked for reaches the
// library. MoviePilot cannot push to Crychic, so it polls MoviePilot's
// transfer (整理) history and matches records against remembered requests,
// which survive restarts in a JSON file.
package notify

import (
	"context"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

// Transfer is one file MoviePilot moved into the library.
type Transfer struct {
	ID       int
	Source   string
	MediaID  string
	Season   *int // nil for movies
	Episodes []int
	Image    string
}

// Feed is where arrivals are read from.
type Feed interface {
	// LatestTransfer is the newest transfer id, 0 if there are none.
	LatestTransfer(ctx context.Context) (int, error)
	// TransfersAfter lists successful transfers newer than id, oldest first.
	TransfersAfter(ctx context.Context, id int) ([]Transfer, error)
	SubscriptionActive(ctx context.Context, id int) (bool, error)
	// Library and Latest ask the media server what it shows.
	Library(ctx context.Context, media flow.Media) (flow.Library, error)
	Latest(ctx context.Context) ([]flow.LibraryItem, error)
}

// Sender delivers a notice to a requester on their platform.
type Sender interface {
	Notify(ctx context.Context, n flow.Notice) error
}
