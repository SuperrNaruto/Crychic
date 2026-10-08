package flow

import (
	"context"
	"fmt"
	"slices"
)

const (
	actionBack = "bk" // shows the screen before the current one again

	// maxHistory bounds how many screens 返回 can walk back through.
	maxHistory = 20

	// ticketedParts is button data with a ticket: session, action, arg, ticket.
	ticketedParts = 4
)

// screen is a reply once shown and the session state behind it, so 返回
// can show it again exactly as it was.
type screen struct {
	state *session
	reply Reply
	chart bool // a chart page, shown afresh so its missing synopses are retried
	list  bool // a list a pick was made from, which a finished request leads back to
}

// chartViews are the steps that show a chart page.
var chartViews = map[string]bool{actionChart: true, actionChartPage: true, actionWeekday: true}

// forward are the steps that lead on from a screen, which 返回 leads back
// to; same-level steps (paging, ticking, a weekday) only replace the screen,
// and every other step starts afresh.
var forward = map[string]bool{
	actionMedia: true, actionSeason: true, actionRelated: true, actionSeries: true,
	actionMulti: true, actionAskFrom: true, actionChartPick: true, actionAskTitle: true,
	actionResearch: true, actionAskCancel: true, actionCancelPick: true,
	actionHistory: true, actionHistoryPick: true, actionSubDetail: true,
	actionTorrentRun: true, actionTorrentPick: true, actionTorrentMulti: true, actionTorrentBatch: true,
}

var sameLevel = map[string]bool{
	actionTick: true, actionChartPage: true, actionWeekday: true, actionPage: true, answered: true,
	actionRetry: true, actionSubsKind: true, actionHistoryKind: true, actionRefreshSubDetail: true,
	actionPauseSub: true, actionResumeSub: true,
	actionTorrentAgain: true, actionTorrentSort: true, actionTorrentSites: true, actionTorrentSite: true,
	actionTorrentTick: true, actionTorrentMissing: true,
}

// listPicks are the forward steps that pick from a list.
var listPicks = map[string]bool{actionMedia: true, actionChartPick: true}

// finishing are the steps that write a request; one picked from a list
// leads back to that list afterwards, so browsing goes on from there.
var finishing = map[string]bool{
	actionConfirm: true, actionSubscribed: true, actionTorrentGet: true, actionTorrentBatchGet: true,
}

// needsTicket also binds subscription mutations to the currently shown
// screen; those writes do not finish a media request.
func needsTicket(action string) bool {
	return finishing[action] || action == actionPauseSub || action == actionResumeSub || action == actionUnsubscribe
}

// answered stands for a typed start episode, which replaces its prompt.
const answered = "typed"

// navigate records reply as the session's current screen after a step,
// pushing the screen it replaced when the step led forward, and offers 返回
// while there is somewhere to go back to. Notices leave the screen as it
// was, and so do steps that ended the conversation. A reply that follows
// a resource search is not a screen to go back to; the card it skipped
// (huntNow) is recorded in its place.
func (e *Engine) navigate(prev session, action string, reply Reply) Reply {
	if reply.Notice != "" {
		return reply
	}
	next, ok := e.store.get(prev.id)
	if !ok {
		return reply
	}
	if reply.Follow == "" {
		return e.record(prev, action, next, reply)
	}
	if skipped := next.skipped; skipped != nil {
		next.skipped = nil
		e.record(prev, action, next, *skipped)
	}
	return reply
}

// record makes reply next's current screen once action led on from prev,
// and returns it with the buttons its place in the history adds.
func (e *Engine) record(prev session, action string, next session, reply Reply) Reply {
	next.history = moved(prev, action, next.history)
	if finishing[action] && len(next.history) > 0 {
		reply.Buttons = append(reply.Buttons, browseRow(next.id))
	}
	if len(next.history) > 0 {
		reply = withBack(next.id, reply)
	}
	next.menu = false
	reply = ticketed(&next, reply)
	next.screen = screen{state: snapshot(next), reply: reply, chart: chartViews[action]}
	e.store.put(next)
	return reply
}

// moved is the history once action led on from prev; kept is the history
// the step itself left.
func moved(prev session, action string, kept []screen) []screen {
	switch {
	case forward[action] && prev.screen.state != nil:
		s := prev.screen
		s.list = listPicks[action]
		return pushed(prev.history, s)
	case finishing[action]:
		return toList(prev.history)
	case !forward[action] && !sameLevel[action]:
		return nil
	}
	return kept
}

// toList is history up to the latest list a pick was made from; nil when
// the request never came from one.
func toList(history []screen) []screen {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].list {
			return history[:i+1]
		}
	}
	return nil
}

// settle ends a request's confirmable step before its write, so a second
// tap only flashes a notice: a request picked from a list keeps its session
// for 返回 to lead back there, any other ends with the write. ok is false
// when a tap got there first.
func (e *Engine) settle(sess session) bool {
	if toList(sess.history) == nil {
		_, ok := e.store.take(sess.id)
		return ok
	}
	sess.target, sess.chosen = nil, nil
	e.store.put(sess)
	return true
}

// shown records the first screen of a conversation.
func (e *Engine) shown(id uint64, reply Reply) Reply {
	return e.navigate(session{id: id}, "", reply)
}

// back shows the previous screen with the state it had; a chart page is
// shown afresh from that state.
func (e *Engine) back(ctx context.Context, sess session) Reply {
	if len(sess.history) == 0 {
		return Reply{Notice: msgInvalidChoice}
	}
	last := sess.history[len(sess.history)-1]
	restored := *last.state
	restored.history, restored.screen = sess.history[:len(sess.history)-1], last
	restored.tickets = sess.tickets
	e.store.put(restored)
	if last.chart {
		return e.navigate(restored, actionChartPage, e.chartPage(ctx, restored, restored.page))
	}
	return last.reply
}

// ticketed gives the guarded write buttons of the screen sess now shows
// a fresh ticket, and only a press carrying the shown screen's ticket
// writes. A request picked from a list keeps its session past the write, so
// without one a stale 确认订阅 from before would confirm whatever was picked
// next; 返回 restores a screen with the ticket its buttons carry.
func ticketed(sess *session, reply Reply) Reply {
	sess.ticket = 0
	rows := make([][]Button, 0, len(reply.Buttons))
	for _, row := range reply.Buttons {
		next := slices.Clone(row)
		for i, b := range next {
			id, p, ok := parseData(b.Data)
			if !ok || id != sess.id || !needsTicket(p.action) {
				continue
			}
			if sess.ticket == 0 {
				sess.tickets++
				sess.ticket = sess.tickets
			}
			next[i].Data = fmt.Sprintf("%s:%d", b.Data, sess.ticket)
		}
		rows = append(rows, next)
	}
	if sess.ticket != 0 {
		reply.Buttons = rows
	}
	return reply
}

// pushed is history with s on top, the oldest dropped beyond maxHistory;
// it never writes into history's array, which other sessions may share.
func pushed(history []screen, s screen) []screen {
	kept := history[max(len(history)-maxHistory+1, 0):]
	return append(slices.Clip(kept), s)
}

// snapshot is sess without its own navigation, which would nest.
func snapshot(sess session) *session {
	sess.history, sess.screen = nil, screen{}
	return &sess
}

// labelBack is the one name of every way back, whether it restores the
// screen before (actionBack) or reads a list afresh (backTo).
const labelBack = "返回"

// backTo leads back to a list read afresh, e.g. tasks that moved on.
func backTo(id uint64, action string, arg int) Button {
	return Button{Label: labelBack, Data: data(id, action, arg)}
}

// closeButton ends a browsing screen; 取消 ends a request instead.
func closeButton(id uint64) Button {
	return Button{Label: "关闭", Data: data(id, actionClose, 0)}
}

// browseRow is the last row of a browsing screen: 首页 and 关闭, with 返回
// put in front of them by navigate (or by the screen itself).
func browseRow(id uint64) []Button {
	return []Button{homeButton(id), closeButton(id)}
}

// titled opens a reply with heading unless it already has one.
func titled(heading string, reply Reply) Reply {
	if len(reply.Text) > 0 && reply.Text[0].Kind == Title {
		return reply
	}
	reply.Text = append(Lines(Heading(Plain(heading))), reply.Text...)
	return reply
}

// withBack puts 返回 at the front of the last row's 取消, 首页 or 关闭, or on
// a row of its own; a reply with a 返回 of its own keeps that one.
func withBack(id uint64, reply Reply) Reply {
	back := Button{Label: labelBack, Data: data(id, actionBack, 0)}
	if hasLabel(reply.Buttons, labelBack) {
		return reply
	}
	ends := map[Button]bool{cancelButton(id): true, homeButton(id): true, closeButton(id): true}
	rows := make([][]Button, 0, len(reply.Buttons)+1)
	placed := false
	for _, row := range reply.Buttons {
		next := make([]Button, 0, len(row)+1)
		for _, b := range row {
			if ends[b] && !placed {
				next, placed = append(next, back), true
			}
			next = append(next, b)
		}
		rows = append(rows, next)
	}
	if !placed {
		rows = append(rows, []Button{back})
	}
	reply.Buttons = rows
	return reply
}

func hasLabel(rows [][]Button, label string) bool {
	for _, row := range rows {
		for _, b := range row {
			if b.Label == label {
				return true
			}
		}
	}
	return false
}

// grid lays buttons out columns to a row.
func grid(buttons []Button, columns int) [][]Button {
	var rows [][]Button
	for i := 0; i < len(buttons); i += columns {
		rows = append(rows, buttons[i:min(i+columns, len(buttons))])
	}
	return rows
}
