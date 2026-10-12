package e2e

import (
	"fmt"
	"net/http"
)

// chatAction records a sendChatAction like Telegram would accept it: the
// chat must exist, a topic must belong to a forum, and only the action the
// bot uses is known to the fake.
func (f *fakeTelegram) chatAction(w http.ResponseWriter, r *http.Request) {
	chat, err := resolveChat(r.FormValue("chat_id"))
	if err != nil {
		reply(w, nil, fmt.Errorf("chat not found"))
		return
	}
	if action := r.FormValue("action"); action != "typing" {
		reply(w, nil, fmt.Errorf("wrong parameter action in request: %q", action))
		return
	}
	to, err := f.addressed(r, "sendChatAction", chat)
	if err != nil {
		reply(w, nil, err)
		return
	}
	head := fmt.Sprintf("<< sendChatAction chat=%d", chat)
	if to.forum {
		head += fmt.Sprintf(" topic=%d", to.thread)
	}
	f.tr.add(head + " typing")
	f.finish(fmt.Sprintf("action:%d", chat))
	reply(w, true, nil)
}
