package flow

import (
	"context"
	"slices"
)

// opener shows one browsing page in a fresh session.
type opener func(ctx context.Context, sess session) Reply

// revive reopens a stateless browsing page whose conversation expired or
// was lost to a restart, in a new session owned by actor. Only a private
// chat revives, and only pages that need no earlier state: home, a home
// feature, close, and the back buttons of the task, subscription and
// chart menus. A cancelled ctx (shutdown) and a ticketed press never revive.
func (e *Engine) revive(ctx context.Context, actor Actor, p press) (Reply, bool) {
	if !actor.Private || ctx.Err() != nil || p.ticket != 0 {
		return Reply{}, false
	}
	if p.action == actionClose {
		return closedReply(), true
	}
	open := e.reviveOpener(p)
	if open == nil {
		return Reply{}, false
	}
	sess := e.store.create(actor, nil)
	return e.shown(sess.id, open(ctx, sess)), true
}

// reviveOpener is the page a stateless press reopens; nil for any press
// that needs its old conversation. Features open as the typed commands do,
// so an empty list is a message that replaces the expired one.
func (e *Engine) reviveOpener(p press) opener {
	switch p.action {
	case actionHome:
		return func(_ context.Context, sess session) Reply { return e.home(sess) }
	case actionFeature:
		features := e.features()
		if p.arg < 0 || p.arg >= len(features) {
			return nil
		}
		return features[p.arg].open
	case actionList:
		return e.listTasks
	case actionSubs:
		return e.listSubs
	case actionSubsKind:
		if !slices.Contains(subKinds, Kind(p.arg)) {
			return nil
		}
		return func(ctx context.Context, sess session) Reply {
			sess.kind = Kind(p.arg)
			return e.listSubs(ctx, sess)
		}
	case actionCharts:
		return func(_ context.Context, sess session) Reply { return e.chartMenu(sess) }
	}
	return nil
}
