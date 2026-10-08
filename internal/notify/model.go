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
	Date     string // MoviePilot's completion time, YYYY-MM-DD HH:MM:SS in its local time
	Source   string
	MediaID  string
	Title    string
	Year     string
	Season   *int // nil for movies
	Episodes []int
	Image    string
	File     string // the downloaded file's name, "" when unknown
	Download string // the download (torrent hash) the file came from
}

// Quality is how the backend's release-name parser words a file, e.g.
// 1080p, BluRay, x265, FLAC 2.0, FROGE; empty fields are unknown.
type Quality struct {
	Resolution string
	Edition    string
	WebSource  string
	Video      string
	Audio      string
	Group      string
}

// Feed is where arrivals are read from.
type Feed interface {
	// LatestTransfer includes every successful id in the newest completion
	// second; its date is empty when there are no successful transfers.
	LatestTransfer(ctx context.Context) (TransferCursor, error)
	// TransfersSince includes date's whole second, oldest first. An empty
	// date reads the bounded catch-up window when migrating an old state.
	TransfersSince(ctx context.Context, date string) ([]Transfer, error)
	// Subscription reads the current row, whose target may differ from the
	// watched one after id reuse; an empty subscription when it is gone.
	Subscription(ctx context.Context, id int) (flow.Subscription, error)
	// Library and Latest ask the media server what it shows.
	Library(ctx context.Context, media flow.Media) (flow.Library, error)
	Latest(ctx context.Context) ([]flow.LibraryItem, error)
	// Downloads lists the downloader's unfinished tasks.
	Downloads(ctx context.Context) ([]flow.Download, error)
	// Quality parses a downloaded file's name; a zero Quality when the
	// backend cannot recognize it.
	Quality(ctx context.Context, file string) (Quality, error)
}

// Sender delivers a notice to a requester on their platform.
type Sender interface {
	Notify(ctx context.Context, n flow.Notice) error
}
