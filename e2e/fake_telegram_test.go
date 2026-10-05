package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	tgToken      = "123:test-token"
	botUserID    = 999
	messageDate  = 1_760_000_000
	maxMultipart = 1 << 20
)

type button struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type message struct {
	chat int64
	text string
	rows [][]button
}

// fakeTelegram is a Bot API server: it hands queued updates to getUpdates,
// keeps the messages the bot sends or edits, and signals when the call that
// completes a user action arrives.
type fakeTelegram struct {
	*httptest.Server
	tr *transcript

	mu        sync.Mutex
	updates   []map[string]any
	confirmed int
	arrived   chan struct{}
	messages  map[int]message
	lastMsg   int
	waiters   map[string]chan struct{}
}

func newFakeTelegram(tr *transcript) *fakeTelegram {
	f := &fakeTelegram{
		tr:       tr,
		arrived:  make(chan struct{}),
		messages: map[int]message{},
		waiters:  map[string]chan struct{}{},
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	return f
}

// push queues an update and returns a channel closed when the bot finishes
// handling it, identified by the completing call's key; an empty key means
// the bot is expected to stay silent.
func (f *fakeTelegram) push(upd map[string]any, doneKey string) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	done := make(chan struct{})
	if doneKey != "" {
		f.waiters[doneKey] = done
	}
	upd["update_id"] = len(f.updates) + 1
	f.updates = append(f.updates, upd)
	close(f.arrived)
	f.arrived = make(chan struct{})
	return done
}

// expect returns a channel closed when the call identified by key arrives,
// for bot output that no update triggers (notices).
func (f *fakeTelegram) expect(key string) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	done := make(chan struct{})
	f.waiters[key] = done
	return done
}

func (f *fakeTelegram) message(id int) (message, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.messages[id]
	return m, ok
}

func (f *fakeTelegram) serve(w http.ResponseWriter, r *http.Request) {
	method, found := strings.CutPrefix(r.URL.Path, "/bot"+tgToken+"/")
	if !found {
		reply(w, nil, fmt.Errorf("unknown path %s", r.URL.Path))
		return
	}
	_ = r.ParseMultipartForm(maxMultipart)
	switch method {
	case "getMe":
		reply(w, map[string]any{"id": botUserID, "is_bot": true, "first_name": "Crychic", "username": "crychic_bot"}, nil)
	case "getUpdates":
		f.getUpdates(w, r)
	case "sendMessage", "editMessageText":
		reply(w, f.store(r, method), nil)
	case "answerCallbackQuery":
		f.answer(w, r)
	default:
		reply(w, nil, fmt.Errorf("method %s not supported by fake", method))
	}
}

// getUpdates long-polls like Telegram: it returns unconfirmed updates, or
// waits for one until the poll timeout or disconnect.
func (f *fakeTelegram) getUpdates(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.Atoi(r.FormValue("offset"))
	timeout, _ := strconv.Atoi(r.FormValue("timeout"))
	deadline := time.After(time.Duration(timeout) * time.Second)
	for {
		f.mu.Lock()
		// Like Telegram, an offset confirms every earlier update for good,
		// so a restarted bot never sees them again.
		f.confirmed = max(f.confirmed, offset-1)
		pending := f.updates[min(f.confirmed, len(f.updates)):]
		arrived := f.arrived
		f.mu.Unlock()
		if len(pending) > 0 {
			reply(w, pending, nil)
			return
		}
		select {
		case <-arrived:
		case <-deadline:
			reply(w, []any{}, nil)
			return
		case <-r.Context().Done():
			return
		}
	}
}

// store records a sent or edited message and returns it as Telegram would.
func (f *fakeTelegram) store(r *http.Request, method string) map[string]any {
	chat, _ := strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
	var markup struct {
		InlineKeyboard [][]button `json:"inline_keyboard"`
	}
	if raw := r.FormValue("reply_markup"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &markup)
	}
	id, _ := strconv.Atoi(r.FormValue("message_id"))
	f.mu.Lock()
	if method == "sendMessage" {
		f.lastMsg++
		id = f.lastMsg
	}
	f.messages[id] = message{chat: chat, text: r.FormValue("text"), rows: markup.InlineKeyboard}
	f.mu.Unlock()

	head := fmt.Sprintf("<< %s chat=%d message=%d", method, chat, id)
	if mode := r.FormValue("parse_mode"); mode != "" {
		head += " parse_mode=" + mode
	}
	var preview struct {
		URL   string `json:"url"`
		Large bool   `json:"prefer_large_media"`
		Above bool   `json:"show_above_text"`
	}
	_ = json.Unmarshal([]byte(r.FormValue("link_preview_options")), &preview)
	if preview.URL != "" {
		head += fmt.Sprintf(" poster=%s large=%t above=%t", preview.URL, preview.Large, preview.Above)
	}
	doneKey := fmt.Sprintf("edit:%d", id)
	if method == "sendMessage" {
		doneKey = fmt.Sprintf("send:%d", chat)
	}
	body := []string{r.FormValue("text")}
	if len(markup.InlineKeyboard) > 0 {
		body = append(body, buttonLines(markup.InlineKeyboard))
	}
	f.tr.add(head, body...)
	f.finish(doneKey)
	return map[string]any{
		"message_id": id, "date": messageDate, "text": r.FormValue("text"),
		"chat": map[string]any{"id": chat, "type": chatType(chat)},
	}
}

func (f *fakeTelegram) answer(w http.ResponseWriter, r *http.Request) {
	head := "<< answerCallbackQuery"
	if text := r.FormValue("text"); text != "" {
		head += " notice=" + strconv.Quote(text)
	}
	f.tr.add(head)
	f.finish("callback:" + r.FormValue("callback_query_id"))
	reply(w, true, nil)
}

func (f *fakeTelegram) finish(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if done, ok := f.waiters[key]; ok {
		close(done)
		delete(f.waiters, key)
	}
}

func reply(w http.ResponseWriter, result any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": http.StatusNotFound, "description": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

func chatType(chat int64) string {
	if chat < 0 {
		return "group"
	}
	return "private"
}
