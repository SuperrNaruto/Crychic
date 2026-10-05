// Package e2e drives the real Crychic app, wired exactly as main wires it,
// against a fake Telegram Bot API and a fake MoviePilot. Scenarios speak in
// user actions and assert on the full recorded transcript.
package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SuperrNauto/Crychic/internal/app"
	"github.com/SuperrNauto/Crychic/internal/config"
)

const (
	alice    int64 = 1001
	bob      int64 = 1002
	stranger int64 = 2002
	group    int64 = -500

	actionTimeout = 5 * time.Second
	// notifyInterval keeps arrival polling fast enough for tests.
	notifyInterval = "100ms"
	// notifyQuiet is the default settling time for a show's arrivals.
	notifyQuiet = "300ms"
	// progressInterval keeps live task views refreshing fast enough for tests.
	progressInterval = "100ms"
)

type scenario struct {
	routes  map[string]route
	apiKey  string
	history []transfer // transfers that predate the bot
	quiet   string     // CRYCHIC_NOTIFY_QUIET, notifyQuiet if empty
}

type harness struct {
	t      *testing.T
	tr     *transcript
	tg     *fakeTelegram
	mp     *fakeMoviePilot
	cfg    config.Config
	stop   func()
	nextCB int
}

// start boots the app with env-style configuration, as a deployment would,
// and returns once the arrival notifier has read its baseline.
func start(t *testing.T, sc scenario) *harness {
	t.Helper()
	tr := &transcript{}
	mp := newFakeMoviePilot(t, tr, sc.routes)
	mp.add(sc.history...)
	tg := newFakeTelegram(tr)
	if sc.apiKey == "" {
		sc.apiKey = mpAPIKey
	}
	if sc.quiet == "" {
		sc.quiet = notifyQuiet
	}
	env := map[string]string{
		"CRYCHIC_MOVIEPILOT_URL":         mp.URL,
		"CRYCHIC_MOVIEPILOT_API_KEY":     sc.apiKey,
		"CRYCHIC_TELEGRAM_TOKEN":         tgToken,
		"CRYCHIC_TELEGRAM_API_URL":       tg.URL,
		"CRYCHIC_TELEGRAM_ALLOWED_USERS": fmt.Sprintf("%d, %d", alice, bob),
		"CRYCHIC_DATA_DIR":               t.TempDir(),
		"CRYCHIC_NOTIFY_INTERVAL":        notifyInterval,
		"CRYCHIC_NOTIFY_QUIET":           sc.quiet,
		"CRYCHIC_PROGRESS_INTERVAL":      progressInterval,
	}
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, tr: tr, tg: tg, mp: mp, cfg: cfg}
	h.launch()
	h.wait(mp.polled, "the notifier's first poll")
	t.Cleanup(func() {
		h.stop()
		tg.Close()
		mp.Close()
	})
	return h
}

func (h *harness) launch() {
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	log := slog.New(slog.NewTextHandler(testWriter{h.t}, nil))
	go func() { stopped <- app.Run(ctx, h.cfg, log) }()
	h.stop = func() {
		cancel()
		if err := <-stopped; err != nil {
			h.t.Errorf("app.Run: %v", err)
		}
	}
}

// restart stops the app and starts it again on the same data directory.
func (h *harness) restart() {
	h.t.Helper()
	h.tr.add(">> Crychic restarts")
	h.stop()
	h.launch()
}

// arrives adds records to MoviePilot's transfer history and waits for the
// bot to post a notice in chat.
func (h *harness) arrives(chat int64, records ...transfer) {
	h.t.Helper()
	h.logTransfers(records)
	done := h.tg.expect(fmt.Sprintf("send:%d", chat))
	h.mp.add(records...)
	h.wait(done, "an arrival notice")
}

// transfers adds records to MoviePilot's transfer history and waits until
// the notifier has read them, without expecting a notice.
func (h *harness) transfers(records ...transfer) {
	h.t.Helper()
	h.logTransfers(records)
	h.wait(h.mp.add(records...), "the notifier reading new transfers")
}

func (h *harness) logTransfers(records []transfer) {
	for _, r := range records {
		h.tr.add(strings.TrimSpace(fmt.Sprintf(">> MoviePilot transfers %s (%s/%s) %s%s", r.Title, r.MediaSource, r.MediaID, r.Seasons, r.Episodes)))
	}
}

// reports makes MoviePilot answer key with fixture from now on and waits
// for the live view in message msgID to show the change.
func (h *harness) reports(msgID int, key, fixture string) {
	h.t.Helper()
	h.tr.add(fmt.Sprintf(">> MoviePilot now answers %s with %s", key, fixture))
	done := h.tg.expect(fmt.Sprintf("edit:%d", msgID))
	h.mp.setRoute(key, ok(fixture))
	h.wait(done, "a live view refresh")
}

// say sends a text message from user in chat and waits for the bot's reply.
func (h *harness) say(user, chat int64, text string) {
	h.t.Helper()
	h.tr.add(fmt.Sprintf(">> user %d in chat %d: %s", user, chat, text))
	h.wait(h.tg.push(chatMessage{user: user, chat: chat, text: text}.update(), fmt.Sprintf("send:%d", chat)), text)
}

// answer types text into message msgID's chat and waits for the bot to
// update that message.
func (h *harness) answer(user int64, msgID int, text string) {
	h.t.Helper()
	h.typeInto(chatMessage{user: user, chat: mustMessage(h, msgID).chat, text: text}, msgID)
}

// answerQuoting is answer as a reply quoting message msgID, which is how
// group members answer.
func (h *harness) answerQuoting(user int64, msgID int, text string) {
	h.t.Helper()
	h.typeInto(chatMessage{user: user, chat: mustMessage(h, msgID).chat, text: text, replyTo: msgID}, msgID)
}

func (h *harness) typeInto(m chatMessage, msgID int) {
	h.t.Helper()
	how := ""
	if m.replyTo > 0 {
		how = fmt.Sprintf(" quoting message %d", m.replyTo)
	}
	h.tr.add(fmt.Sprintf(">> user %d in chat %d%s: %s", m.user, m.chat, how, m.text))
	h.wait(h.tg.push(m.update(), fmt.Sprintf("edit:%d", msgID)), m.text)
}

// chatter sends a message the bot must ignore. Nothing waits for it; a
// stray bot reaction shows up in the transcript.
func (h *harness) chatter(user, chat int64, text string) {
	h.t.Helper()
	h.tr.add(fmt.Sprintf(">> user %d in chat %d: %s", user, chat, text))
	h.tg.push(chatMessage{user: user, chat: chat, text: text}.update(), "")
}

// chatMessage is a text message a user sends, optionally replying to one
// of the bot's messages.
type chatMessage struct {
	user, chat int64
	text       string
	replyTo    int
}

func (m chatMessage) update() map[string]any {
	msg := map[string]any{
		"message_id": 0, "date": messageDate, "text": m.text,
		"from": map[string]any{"id": m.user, "is_bot": false, "first_name": strconv.FormatInt(m.user, 10)},
		"chat": map[string]any{"id": m.chat, "type": chatType(m.chat)},
	}
	if m.replyTo > 0 {
		msg["reply_to_message"] = map[string]any{
			"message_id": m.replyTo, "date": messageDate,
			"chat": map[string]any{"id": m.chat, "type": chatType(m.chat)},
		}
	}
	return map[string]any{"message": msg}
}

// tap presses the button labelled label on message msgID, failing the test
// if the message does not currently show that button.
func (h *harness) tap(user int64, msgID int, label string) {
	h.t.Helper()
	msg := mustMessage(h, msgID)
	data, ok := findButton(msg.rows, label)
	if !ok {
		h.t.Fatalf("message %d has no button %q; it shows:\n%s", msgID, label, buttonLines(msg.rows))
	}
	h.tapData(user, msgID, data)
}

// tapData presses a button by its raw data, e.g. one no longer on screen.
func (h *harness) tapData(user int64, msgID int, data string) {
	h.t.Helper()
	msg, _ := h.tg.message(msgID)
	h.nextCB++
	id := strconv.Itoa(h.nextCB)
	h.tr.add(fmt.Sprintf(">> user %d taps %s on message %d", user, data, msgID))
	done := h.tg.push(map[string]any{"callback_query": map[string]any{
		"id": id, "data": data, "chat_instance": "ci",
		"from": map[string]any{"id": user, "is_bot": false, "first_name": strconv.FormatInt(user, 10)},
		"message": map[string]any{
			"message_id": msgID, "date": messageDate, "text": msg.text,
			"chat": map[string]any{"id": msg.chat, "type": chatType(msg.chat)},
		},
	}}, "callback:"+id)
	h.wait(done, data)
}

// shows asserts message msgID's current text contains want.
func (h *harness) shows(msgID int, want string) {
	h.t.Helper()
	msg, _ := h.tg.message(msgID)
	if !strings.Contains(msg.text, want) {
		h.t.Fatalf("message %d shows %q, want it to contain %q", msgID, msg.text, want)
	}
}

func (h *harness) wait(done <-chan struct{}, action string) {
	h.t.Helper()
	select {
	case <-done:
	case <-time.After(actionTimeout):
		h.t.Fatalf("bot did not finish handling %q\n--- transcript ---\n%s", action, h.tr)
	}
}

func findButton(rows [][]button, label string) (string, bool) {
	for _, row := range rows {
		for _, b := range row {
			if b.Text == label {
				return b.CallbackData, true
			}
		}
	}
	return "", false
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}
