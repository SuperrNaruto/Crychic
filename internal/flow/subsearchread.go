package flow

import "context"

func (e *Engine) refreshSubSearch(ctx context.Context, sess session) Reply {
	sess = e.readSubSearch(ctx, sess)
	return e.showSubSearch(sess)
}

func (e *Engine) followSubSearch(ctx context.Context, sess session) Reply {
	if !sess.subSearch.on {
		return Reply{Notice: msgNotFollowing}
	}
	sess = e.readSubSearch(ctx, sess)
	// A callback cancels its in-flight follower before using the shown
	// screen. Discard that interrupted read, not the callback's controls.
	if ctx.Err() != nil {
		return Reply{Notice: msgNotFollowing}
	}
	return e.showSubSearch(sess)
}

// Reading a locally received batch ID is safe even after that subscription
// has ended. Cancellation separately rechecks its current identity and owner.
func (e *Engine) readSubSearch(ctx context.Context, sess session) session {
	run := sess.subSearch
	if run.batchID == "" {
		run.on = false
		return sess
	}
	ctx, cancel := context.WithTimeout(ctx, cosmeticTimeout)
	defer cancel()
	batch, err := e.backend.SubscriptionSearchBatch(ctx, run.batchID)
	run.fresh, run.problem = false, ""
	if err != nil {
		run.problem, run.on = msgSubSearchUnread, false
		if message, safe := UserMessage(err); safe {
			run.problem = message
		}
		return sess
	}
	if !run.matches(batch) {
		run.problem, run.on = msgSubSearchScope, false
		return sess
	}
	run.last, run.created, run.fresh = &batch, batch.Created, true
	if !batch.active() || e.now().After(run.until) {
		run.on = false
	}
	return sess
}

func (e *Engine) showSubSearch(sess session) Reply {
	sess.subSearchPage, sess.subSearchAsk = subSearchStatusPage, nil
	sess.pages, sess.listing = nil, ""
	e.store.put(sess)
	return subSearchView(sess)
}

func (e *Engine) askStopSubSearch(ctx context.Context, sess session) Reply {
	sess = e.readSubSearch(ctx, sess)
	if !sess.subSearch.canStop() {
		return e.showSubSearch(sess)
	}
	if _, notice := e.ownedSearchSubscription(ctx, sess, sess.subSearch.base); notice != "" {
		return Reply{Notice: notice}
	}
	sess.subSearch.on, sess.subSearchPage = false, subSearchStopPage
	e.store.put(sess)
	return subSearchStopConfirmation(sess)
}

func (e *Engine) stopSubSearch(ctx context.Context, sess session) Reply {
	if sess.subSearchPage != subSearchStopPage || sess.subSearch.stop != "" {
		return Reply{Notice: msgInvalidChoice}
	}
	sess = e.readSubSearch(ctx, sess)
	run := sess.subSearch
	if !run.canStop() {
		return e.showSubSearch(sess)
	}
	if _, notice := e.ownedSearchSubscription(ctx, sess, run.base); notice != "" {
		return Reply{Notice: notice}
	}
	// Even an explicitly rejected cancellation consumes this confirmation.
	// An unknown result only permits reads, never another PUT from the receipt.
	run.stop, run.on, run.fresh = searchUnknown, false, false
	e.store.put(sess)
	err := e.backend.CancelSubscriptionSearch(ctx, run.batchID)
	sess = copySubSearch(sess)
	run = sess.subSearch
	if err == nil {
		run.stop = searchAccepted
	} else if message, safe := UserMessage(err); safe {
		run.stop, run.stopNote = searchRefused, message
	}
	return e.showSubSearch(sess)
}
