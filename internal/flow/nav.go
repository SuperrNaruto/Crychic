package flow

import "slices"

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
}

// forward are the steps that lead on from a screen, which 返回 leads back
// to; same-level steps (paging, ticking, a weekday) only replace the screen,
// and every other step starts afresh.
var forward = map[string]bool{
	actionMedia: true, actionSeason: true, actionRelated: true, actionSeries: true,
	actionMulti: true, actionAskFrom: true, actionChartPick: true, actionAskTitle: true,
}

var sameLevel = map[string]bool{
	actionTick: true, actionChartPage: true, actionWeekday: true, answered: true,
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
	next.screen = screen{state: snapshot(next), reply: reply}
	e.store.put(next)
	return reply
}

// shown records the first screen of a conversation.
func (e *Engine) shown(id uint64, reply Reply) Reply {
	return e.navigate(session{id: id}, "", reply)
}

// back shows the previous screen with the state it had.
func (e *Engine) back(sess session) Reply {
	if len(sess.history) == 0 {
		return Reply{Notice: msgInvalidChoice}
	}
	last := sess.history[len(sess.history)-1]
	restored := *last.state
	restored.history, restored.screen = sess.history[:len(sess.history)-1], last
	e.store.put(restored)
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

// withBack puts 返回 before 取消, or on a row of its own when there is none.
func withBack(id uint64, reply Reply) Reply {
	back := Button{Label: "返回", Data: data(id, actionBack, 0)}
	cancel := cancelButton(id)
	rows := make([][]Button, 0, len(reply.Buttons)+1)
	placed := false
	for _, row := range reply.Buttons {
		next := make([]Button, 0, len(row)+1)
		for _, b := range row {
			if b == cancel && !placed {
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

// grid lays buttons out columns to a row.
func grid(buttons []Button, columns int) [][]Button {
	var rows [][]Button
	for i := 0; i < len(buttons); i += columns {
		rows = append(rows, buttons[i:min(i+columns, len(buttons))])
	}
	return rows
}
