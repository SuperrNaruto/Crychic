package telegram

import (
	"strings"
	"time"
)

const (
	// defaultAnswerLead lets a quick edit land before its press is answered,
	// keeping the two in order, while a slow one is answered alongside.
	defaultAnswerLead = 50 * time.Millisecond
	// defaultAnswerDeadline is how long a press waits for its flow before it
	// is answered without text; Telegram rejects answers to queries older
	// than about 15s (seen 2026-10-13: "query is too old").
	defaultAnswerDeadline = 10 * time.Second
	// slowPress is how long a button press may take, from receipt to its
	// answer, before its stages are logged.
	slowPress = 500 * time.Millisecond
	// slowPosters is how long preparing a reply's posters may take before
	// it is logged on its own.
	slowPosters = 200 * time.Millisecond
	// actionField is where callback data names its action.
	actionField = 1
)

// stopwatch splits one handler's time into named stages for the log.
type stopwatch struct {
	start, mark time.Time
	stages      []any
}

func newStopwatch() *stopwatch {
	now := time.Now()
	return &stopwatch{start: now, mark: now}
}

// lap closes the stage that ran since the previous lap.
func (s *stopwatch) lap(stage string) {
	now := time.Now()
	s.stages = append(s.stages, stage+"_ms", now.Sub(s.mark).Milliseconds())
	s.mark = now
}

// logSlowPress names only the action, never the data's ids or arguments.
func (a *adapter) logSlowPress(s *stopwatch, data string) {
	total := time.Since(s.start)
	if total < slowPress {
		return
	}
	attrs := append([]any{"action", actionOf(data), "total_ms", total.Milliseconds()}, s.stages...)
	a.log.Info("slow button press", attrs...)
}

// actionOf is the action code of callback data, without its ids.
func actionOf(data string) string {
	if parts := strings.Split(data, ":"); len(parts) > actionField {
		return parts[actionField]
	}
	return ""
}
