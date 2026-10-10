package flow

import (
	"context"
	"slices"
)

const (
	actionTorrentPlan        = "pl"
	actionTorrentPlanRefresh = "pc"
	torrentPlanView          = "torrent-plan"
	msgTorrentPlan           = "缺集方案"
	msgPlanRecalculate       = "重新计算"
	msgPlanChanged           = "库存或季集范围已变化，我重新计算了方案；请核对后再次确认哦～"
)

func planAvailable(sess session) bool {
	return sess.focus != nil && sess.focus.Media.Kind == TV && sess.focus.Season != nil &&
		(!sess.streaming || sess.completedSearch())
}

func (e *Engine) chooseTorrentPlan(ctx context.Context, sess session, p press) (Reply, bool) {
	switch p.action {
	case actionTorrentPlan, actionTorrentPlanRefresh:
		if !planAvailable(sess) {
			return Reply{Notice: msgInvalidChoice}, true
		}
		next, plan := e.readTorrentPlan(ctx, sess)
		return e.showTorrentPlan(next, plan, ""), true
	}
	return Reply{}, false
}

func (e *Engine) showTorrentPlan(sess session, plan *torrentPlan, notice string) Reply {
	sess.plan, sess.batch = plan, slices.Clone(plan.batch)
	sess.searchView, sess.release = torrentPlanView, nil
	return e.listPages(sess, torrentPlanList(sess, notice))
}

// Confirmation rechecks inventory, never searches sites or silently changes
// the accepted batch. A changed scope returns through a read-only transition.
func (e *Engine) confirmTorrentPlan(ctx context.Context, sess session) Reply {
	if sess.plan == nil {
		return e.navigate(sess, actionTorrentBatchGet, e.downloadBatch(ctx, sess))
	}
	if !planAvailable(sess) || sess.plan.problem != "" || len(sess.batch) == 0 {
		return Reply{Notice: msgInvalidChoice}
	}
	next, plan := e.readTorrentPlan(ctx, sess)
	if plan.problem != "" || !samePlanScope(sess.plan.scope, plan.scope) {
		notice := ""
		if plan.problem == "" {
			notice = msgPlanChanged
		}
		return e.navigate(sess, actionTorrentPlanRefresh, e.showTorrentPlan(next, plan, notice))
	}
	return e.navigate(sess, actionTorrentBatchGet, e.downloadBatch(ctx, next))
}

func samePlanScope(a, b torrentPlanScope) bool {
	return a.count == b.count && a.from == b.from && slices.Equal(a.held, b.held)
}

// While a plan is displayed only its own controls can act. In particular,
// stale manual batch controls cannot replace the displayed proposal.
func planScreenChoice(sess session, raw, action string) (bool, bool) {
	if sess.searchView == torrentPlanView || action == actionTorrentPlan || action == actionTorrentPlanRefresh {
		return hasData(sess.screen.reply.Buttons, raw), true
	}
	return false, false
}
