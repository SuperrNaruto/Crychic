package e2e

import (
	"fmt"
	"net/http"
	"strconv"
)

// observeForum records the forum metadata Telegram attaches to updates.
// push holds the fake's lock while it adds the update.
func (f *fakeTelegram) observeForum(update map[string]any) {
	m, ok := update["message"].(map[string]any)
	if !ok {
		return
	}
	chat, ok := m["chat"].(map[string]any)
	if ok && chat["is_forum"] == true {
		f.forums[chat["id"].(int64)] = true
	}
}

// addressed keeps edits in their existing topic. A new send with no topic
// selector is outside all forum threads, which Telegram shows in General.
func (f *fakeTelegram) addressed(r *http.Request, method string, chat int64) (message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := message{chat: chat, forum: f.forums[chat]}
	if method == "editMessageText" {
		id, _ := strconv.Atoi(r.FormValue("message_id"))
		m.thread = f.messages[id].thread
		return m, nil
	}
	if raw := r.FormValue("message_thread_id"); raw != "" {
		thread, err := strconv.Atoi(raw)
		if err != nil || thread <= 0 || !m.forum {
			return message{}, fmt.Errorf("message thread not found")
		}
		m.thread = thread
	}
	return m, nil
}

// sayInTopic sends a command from a non-General forum topic.
func (h *harness) sayInTopic(user int64, topic int, text string) {
	h.t.Helper()
	h.tr.add(fmt.Sprintf(">> user %d in chat %d topic %d: %s", user, group, topic, text))
	m := chatMessage{user: user, chat: group, text: text, thread: topic}
	h.wait(h.tg.push(m.update(), fmt.Sprintf("send:%d", group)), text)
}

func (h *harness) showsInTopic(msgID, topic int) {
	h.t.Helper()
	if got := mustMessage(h, msgID).thread; got != topic {
		h.t.Errorf("message %d arrived in topic %d (0 = General), want %d", msgID, got, topic)
	}
}
