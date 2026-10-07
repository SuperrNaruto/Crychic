package flow

import "context"

// Conversation is the platform-neutral entry point for chat front ends.
// Adapters classify input and render Replies; the engine owns navigation,
// validation and backend calls. Arrival notices use notify.Sender separately.
type Conversation interface {
	Start(ctx context.Context, actor Actor, term string) Reply
	Choose(ctx context.Context, actor Actor, data string) Reply
	Answer(ctx context.Context, actor Actor, typed Typed) Reply
	Tasks(ctx context.Context, actor Actor) Reply
	Home(ctx context.Context, actor Actor) Reply
	Charts(ctx context.Context, actor Actor) Reply
	Subscriptions(ctx context.Context, actor Actor) Reply
	Latest(ctx context.Context, actor Actor) Reply
	Upcoming(ctx context.Context, actor Actor) Reply
}
