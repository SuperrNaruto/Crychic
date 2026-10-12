package e2e

import (
	"fmt"
	"testing"
)

const typingTopic = 17

// A title search or command that takes a moment shows "typing…" in the
// chat, and in the forum topic it came from, before its reply arrives; the
// action is never sent after the reply.
func TestSlowSearchShowsTyping(t *testing.T) {
	h := start(t, scenario{
		typingLead: 1, // the held search outlasts it
		routes: map[string]route{
			searchPath: ok("search_dune.json"),
		},
	})
	h.sayWhileSearchHeld(chatMessage{user: alice, chat: alice, text: "沙丘"})
	h.sayWhileSearchHeld(chatMessage{user: alice, chat: group, text: "/search 沙丘", thread: typingTopic})
	h.tr.verify(t)
}

// sayWhileSearchHeld sends m, holds MoviePilot's title search until the chat
// shows the typing action, then lets the reply through.
func (h *harness) sayWhileSearchHeld(m chatMessage) {
	h.t.Helper()
	held := h.mp.holdCalls(searchPath, 1)[0]
	typing := h.tg.expect(fmt.Sprintf("action:%d", m.chat))
	where := fmt.Sprintf("chat %d", m.chat)
	if m.thread > 0 {
		where += fmt.Sprintf(" topic %d", m.thread)
	}
	h.tr.add(fmt.Sprintf(">> user %d in %s: %s", m.user, where, m.text))
	done := h.tg.push(m.update(), fmt.Sprintf("send:%d", m.chat))
	h.wait(held.entered, "the held search")
	h.wait(typing, "the typing action")
	held.allow()
	h.wait(done, m.text)
}
