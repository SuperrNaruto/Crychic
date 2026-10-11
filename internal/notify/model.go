// Package notify forwards MoviePilot native notices to chat platforms.
// MoviePilot decides every notification; this package only transports them.
package notify

import (
	"context"

	"github.com/SuperrNaruto/Crychic/internal/flow"
)

const stateFileMode = 0o600

// Sender delivers a notice to its target on the chat platform.
type Sender interface {
	Notify(ctx context.Context, n flow.Notice) error
}
