package notify

import "context"

const NativeProtocol = 1

// NativeStatus describes a single MP custom channel, never its credentials.
type NativeStatus struct {
	Protocol int    `json:"protocol"`
	Channel  string `json:"channel_id"`
	Stream   string `json:"stream_id"`
	Head     uint64 `json:"head"`
	Active   bool   `json:"active"`
}

// NativeRecipient is MP's resolved platform address, not an API-key owner.
type NativeRecipient struct {
	Kind  string `json:"kind"`
	Chat  string `json:"chat_id"`
	User  string `json:"user_id,omitempty"`
	Topic int    `json:"topic_id,omitempty"`
}

type NativeMessage struct {
	Sequence  uint64          `json:"sequence"`
	ID        string          `json:"id"`
	Platform  string          `json:"platform"`
	Recipient NativeRecipient `json:"recipient"`
	Title     string          `json:"title"`
	Text      string          `json:"text"`
	Image     string          `json:"image,omitempty"`
	Link      string          `json:"link,omitempty"`
}

type NativeBatch struct {
	NativeStatus
	Messages []NativeMessage `json:"messages"`
}

type NativeCursor struct {
	Consumer string `json:"consumer_id"`
	Channel  string `json:"channel_id"`
	Stream   string `json:"stream_id"`
	Sequence uint64 `json:"sequence"`
}

type NativeFeed interface {
	NativeStatus(context.Context) (NativeStatus, error)
	NativeNotifications(context.Context, NativeCursor) (NativeBatch, error)
	NativeAck(context.Context, NativeCursor) error
}
