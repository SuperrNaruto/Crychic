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

// message is a message on screen: a rich message (text is its HTML,
// media its uploaded photos) or a photo with a caption.
type message struct {
	chat  int64
	text  string
	rows  [][]button
	media []photo
	photo *photo
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
	commands  map[string]string // setMyCommands menus by scope, as sent
	stall     string
	stalled   chan struct{}
	badImage  string // an image URL Telegram fails to fetch
}

func newFakeTelegram(tr *transcript) *fakeTelegram {
	f := &fakeTelegram{
		tr:       tr,
		arrived:  make(chan struct{}),
		messages: map[int]message{},
		commands: map[string]string{},
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
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if f.wait(r, method) {
		return
	}
	if f.serveMessage(w, r, method) {
		return
	}
	switch method {
	case "getMe":
		reply(w, map[string]any{"id": botUserID, "is_bot": true, "first_name": "Crychic", "username": "crychic_bot"}, nil)
	case "getUpdates":
		f.getUpdates(w, r)
	case "answerCallbackQuery":
		f.answer(w, r)
	case "setMyCommands":
		f.setCommands(r)
		reply(w, true, nil)
	default:
		reply(w, nil, fmt.Errorf("method %s not supported by fake", method))
	}
}

func (f *fakeTelegram) serveMessage(w http.ResponseWriter, r *http.Request, method string) bool {
	switch method {
	case "sendRichMessage", "editMessageText":
		result, err := f.store(r, method)
		reply(w, result, err)
	case "deleteMessage":
		err := f.deleteMessage(r)
		reply(w, err == nil, err)
	default:
		return false
	}
	return true
}

func (f *fakeTelegram) setCommands(r *http.Request) {
	scope := r.FormValue("scope")
	if scope == "" {
		scope = defaultScope
	}
	f.mu.Lock()
	f.commands[scope] = r.FormValue("commands")
	f.mu.Unlock()
	if scope == defaultScope {
		f.finish(commandsKey)
	}
}

// refuseImage makes media blocks with url fail, as when Telegram cannot
// download a poster.
func (f *fakeTelegram) refuseImage(url string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.badImage = url
}

// stallNext keeps one API request open until its client cancels it.
func (f *fakeTelegram) stallNext(method string) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stall, f.stalled = method, make(chan struct{})
	return f.stalled
}

func (f *fakeTelegram) wait(r *http.Request, method string) bool {
	f.mu.Lock()
	stalled := f.stalled
	match := f.stall == method
	if match {
		f.stall = ""
	}
	f.mu.Unlock()
	if !match {
		return false
	}
	<-r.Context().Done()
	close(stalled)
	return true
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
func (f *fakeTelegram) store(r *http.Request, method string) (map[string]any, error) {
	m, err := f.readMessage(r, method)
	if err != nil {
		return nil, err
	}
	id, _ := strconv.Atoi(r.FormValue("message_id"))
	f.mu.Lock()
	if strings.HasPrefix(method, "send") {
		f.lastMsg++
		id = f.lastMsg
	}
	f.messages[id] = m
	f.mu.Unlock()

	head := fmt.Sprintf("<< %s chat=%d message=%d", method, m.chat, id)
	for _, p := range m.media {
		head += p.description()
	}
	doneKey := fmt.Sprintf("edit:%d", id)
	if strings.HasPrefix(method, "send") {
		doneKey = fmt.Sprintf("send:%d", m.chat)
	}
	body := richLines(m.text)
	if len(m.rows) > 0 {
		body = append(body, buttonLines(m.rows))
	}
	f.tr.add(head, body...)
	f.finish(doneKey)
	return m.wire(id), nil
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

// commandsKey completes when the bot registers its default command menu,
// which it does last on every start; menus stay out of transcripts except
// where asserted.
const (
	commandsKey  = "commands"
	defaultScope = "default"
)

// menu is the command menu registered for scope (defaultScope or a scope
// as JSON), one "/command – description" a line.
func (f *fakeTelegram) menu(scope string) []string {
	f.mu.Lock()
	raw := f.commands[scope]
	f.mu.Unlock()
	var cmds []struct{ Command, Description string }
	_ = json.Unmarshal([]byte(raw), &cmds)
	lines := make([]string, 0, len(cmds))
	for _, c := range cmds {
		lines = append(lines, "/"+c.Command+" – "+c.Description)
	}
	return lines
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
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": http.StatusBadRequest, "description": "Bad Request: " + err.Error()})
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
