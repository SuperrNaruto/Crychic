package flow

import (
	"context"
	"slices"
)

const (
	actionBack = "bk" // shows the screen before the current one again

	// maxHistory bounds how many screens 返回 can walk back through.
	maxHistory = 20
)

// screen is a reply once shown and the session state behind it, so 返回
// can show it again exactly as it was.
type screen struct {
	state *session
	reply Reply
	chart bool // a chart page, shown afresh so its missing synopses are retried
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
	actionTorrentAgain: true, actionTorrentSort: true, actionTorrentSites: true, actionTorrentSite: true,
	actionTorrentTick: true, actionTorrentMissing: true,
}

// answered stands for a typed start episode, which replaces its prompt.
const answered = "typed"

// navigate records reply as the session's current screen after a step,
// pushing the screen it replaced when the step led forward, and offers 返回
// while there is somewhere to go back to. Notices leave the screen as it
// was, and so do steps that ended the conversation.
func (e *Engine) navigate(prev session, action string, reply Reply) Reply {
	if reply.Notice != "" || reply.Follow != "" {
		return reply
	}
	next, ok := e.store.get(prev.id)
	if !ok {
		return reply
	}
	switch {
	case forward[action] && prev.screen.state != nil:
		next.history = pushed(prev.history, prev.screen)
	case !forward[action] && !sameLevel[action]:
		next.history = nil
	}
	if len(next.history) > 0 {
		reply = withBack(next.id, reply)
	}
	next.menu = false
	next.screen = screen{state: snapshot(next), reply: reply, chart: chartViews[action]}
	e.store.put(next)
	return reply
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
	e.store.put(restored)
	if last.chart {
		return e.navigate(restored, actionChartPage, e.chartPage(ctx, restored, restored.page))
	}
	return last.reply
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
