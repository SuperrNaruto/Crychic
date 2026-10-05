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
)

type scenario struct {
	routes map[string]route
	apiKey string
}

type harness struct {
	t      *testing.T
	tr     *transcript
	tg     *fakeTelegram
	nextCB int
}

// start boots the app with env-style configuration, as a deployment would.
func start(t *testing.T, sc scenario) *harness {
	t.Helper()
	tr := &transcript{}
	mp := newFakeMoviePilot(t, tr, sc.routes)
	tg := newFakeTelegram(tr)
	if sc.apiKey == "" {
		sc.apiKey = mpAPIKey
	}
	env := map[string]string{
		"CRYCHIC_MOVIEPILOT_URL":         mp.URL,
		"CRYCHIC_MOVIEPILOT_API_KEY":     sc.apiKey,
		"CRYCHIC_TELEGRAM_TOKEN":         tgToken,
		"CRYCHIC_TELEGRAM_API_URL":       tg.URL,
		"CRYCHIC_TELEGRAM_ALLOWED_USERS": fmt.Sprintf("%d, %d", alice, bob),
	}
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	go func() { stopped <- app.Run(ctx, cfg, log) }()
	t.Cleanup(func() {
		cancel()
		if err := <-stopped; err != nil {
			t.Errorf("app.Run: %v", err)
		}
		tg.Close()
		mp.Close()
	})
	return &harness{t: t, tr: tr, tg: tg}
}

// say sends a text message from user in chat and waits for the bot's reply.
func (h *harness) say(user, chat int64, text string) {
	h.t.Helper()
	h.tr.add(fmt.Sprintf(">> user %d in chat %d: %s", user, chat, text))
	done := h.tg.push(map[string]any{"message": map[string]any{
		"message_id": 0, "date": messageDate, "text": text,
		"from": map[string]any{"id": user, "is_bot": false, "first_name": strconv.FormatInt(user, 10)},
		"chat": map[string]any{"id": chat, "type": chatType(chat)},
	}}, fmt.Sprintf("send:%d", chat))
	h.wait(done, text)
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
