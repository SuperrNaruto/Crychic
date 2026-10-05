// Package flow is the platform-agnostic request conversation: search, pick,
// (pick a season), confirm, subscribe. Chat platforms only render its Replies
// and feed back the opaque button data.
package flow

import (
	"context"
	"errors"
)

// Kind is the media category a subscription is made for.
type Kind int

const (
	Movie Kind = iota + 1
	TV
)

func (k Kind) String() string {
	if k == TV {
		return "电视剧"
	}
	return "电影"
}

// Media identifies one search result by its backend identity.
type Media struct {
	Source   string
	ID       string
	Title    string
	Year     string
	Kind     Kind
	Overview string
}

// Season is one subscribable season of a TV show.
type Season struct {
	Number       int
	Name         string
	EpisodeCount int
}

// Target is what gets subscribed: a movie, or one season of a show.
type Target struct {
	Media  Media
	Season *int
}

// Backend is the media server the bot subscribes through.
type Backend interface {
	Search(ctx context.Context, term string) ([]Media, error)
	Seasons(ctx context.Context, media Media) ([]Season, error)
	IsSubscribed(ctx context.Context, target Target) (bool, error)
	Subscribe(ctx context.Context, target Target) error
}

// Actor is the chat user driving a conversation, in platform terms.
type Actor struct {
	UserID int64
}

// Button is one choice offered to the user; Data comes back to Choose.
type Button struct {
	Label string
	Data  string
}

// Reply is what the platform shows after an action. A Reply with Notice set
// leaves the conversation message untouched and only flashes the notice.
type Reply struct {
	Text    string
	Buttons [][]Button
	Notice  string
}

// UserError carries a message that is safe and useful to show the user.
type UserError struct{ Message string }

func (e *UserError) Error() string { return e.Message }

// UserMessage extracts the user-facing text of err, if it has one.
func UserMessage(err error) (string, bool) {
	var ue *UserError
	if errors.As(err, &ue) {
		return ue.Message, true
	}
	return "", false
}
