package flow

import "context"

// lockSession validates identity before waiting. Re-read after acquiring:
// another action may have consumed or changed the conversation meanwhile.
func (e *Engine) lockSession(ctx context.Context, actor Actor, id uint64) (session, func(), bool) {
	sess, ok := e.store.get(id)
	if !ok || sess.owner.UserID != actor.UserID || sess.owner.Address != actor.Address {
		return sess, func() {}, ok
	}
	select {
	case sess.gate <- struct{}{}:
	case <-ctx.Done():
		return session{}, func() {}, false
	}
	unlock := func() { <-sess.gate }
	current, ok := e.store.get(id)
	return current, unlock, ok
}
