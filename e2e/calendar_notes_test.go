package e2e

import (
	"strings"
	"testing"
	"time"
)

const (
	mondayNoteGrace = 300 * time.Millisecond
	// calendarMissTTL outlasts the flow's hour for a lookup that found no
	// synopsis, yet not the six hours for one that did.
	calendarMissTTL = 2 * time.Hour
	// slowDetails outlasts the calendar card's shorter budget but not the
	// usual one.
	slowDetails      = 4 * time.Second
	calendarCardWait = 3500 * time.Millisecond
	// followIdle keeps a Follow's own cadence out of the way, so only the
	// wake-up at the end of the lookups redraws the page.
	followIdle = "1h"
	// noteSlots is the flow's cap on MoviePilot calls of note lookups.
	noteSlots = 2
	// heldSearches outnumbers the searches Monday's picks make.
	heldSearches = 40
	// timeoutMissTTL passes the flow's ten minutes for a lookup the budget
	// cut short, yet not the hour for one that finished empty.
	timeoutMissTTL = 11 * time.Minute
)

const (
	searchLine  = "-> MoviePilot GET /api/v1/media/search"
	subjectLine = "-> Bangumi GET " + subjectsPath
)

// A calendar page whose synopses are still being looked up is shown at once
// with what is known, and the same message fills them in when the lookups
// end; the entries, numbers and buttons stay where they were.
func TestCalendarPageFollowsItsNotes(t *testing.T) {
	h := start(t, scenario{
		noteGrace: mondayNoteGrace,
		progress:  followIdle,
		routes: map[string]route{
			searchPath:               ok("empty.json"),
			bangumiInfoPath + nuwaID: ok(writeFixture(t, map[string]any{"overview": "MoviePilot 返回的女娲简介"})),
		},
	})
	// One held lookup keeps the round open; it holds one of the few slots,
	// so the other picks' lookups pass through the other one.
	held := h.mp.holdCalls(bangumiInfoPath+nuwaID, 1)
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "新番放送")
	h.shows(1, "简介补充中")
	if strings.Contains(mustMessage(h, 1).text, "MoviePilot 返回的女娲简介") {
		t.Fatal("a synopsis still being looked up was shown")
	}
	buttons := buttonLines(mustMessage(h, 1).rows)
	filled := h.tg.expect("edit:1")
	h.wait(held[0].entered, "the held synopsis lookup")
	held[0].allow()
	h.wait(filled, "the page filling in its synopses")
	h.shows(1, "MoviePilot 返回的女娲简介")
	if strings.Contains(mustMessage(h, 1).text, "简介补充中") {
		t.Error("the page kept saying it was looking up synopses")
	}
	if got := buttonLines(mustMessage(h, 1).rows); got != buttons {
		t.Errorf("buttons changed while the page filled in:\n%s\nwas\n%s", got, buttons)
	}
	h.tr.verify(t)
}

// A page's notes are remembered across weekdays and sessions: showing a day
// again looks nothing up, a lookup that found no synopsis waits an hour
// before another, and one that found it much longer.
func TestCalendarRemembersNotes(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{searchPath: ok("empty.json")}})
	h.bgm.setDown(nuwaID, true)
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "新番放送")
	h.tap(alice, 1, "二")
	lookups := strings.Count(h.tr.String(), searchLine)
	h.tap(alice, 1, "一")
	h.shows(1, "星期一（今天）")
	if got := strings.Count(h.tr.String(), searchLine); got != lookups {
		t.Errorf("showing a day again searched %d more times", got-lookups)
	}
	h.bgm.setDown(nuwaID, false)
	h.advance(calendarMissTTL)
	h.say(alice, alice, "/trending")
	h.tap(alice, 2, "新番放送")
	h.shows(2, "补天的时代已经过去")
	text := h.tr.String()
	if got := strings.Count(text, subjectLine+nuwaID); got != 2 {
		t.Errorf("the synopsis nobody had was looked up %d times, want 2", got)
	}
	if got := strings.Count(text, subjectLine+"418229"); got != 1 {
		t.Errorf("a synopsis found long ago was looked up %d times, want 1", got)
	}
	h.tr.verify(t)
}

// Note lookups share a few MoviePilot calls however many picks a page has:
// with every search held, no more than the cap are in progress at once.
func TestCalendarNotesShareFewMoviePilotCalls(t *testing.T) {
	h := start(t, scenario{noteGrace: mondayNoteGrace, progress: followIdle, routes: map[string]route{searchPath: ok("empty.json")}})
	held := h.mp.holdCalls(searchPath, heldSearches)
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "新番放送")
	filled := h.tg.expect("edit:1")
	for _, call := range held[:noteSlots] {
		h.wait(call.entered, "a held search")
	}
	for _, call := range held[:noteSlots+1] {
		call.allow()
	}
	h.wait(held[noteSlots].entered, "the next search taking a freed slot")
	for _, call := range held {
		call.allow()
	}
	h.wait(filled, "the page filling in its notes")
	if peak := h.mp.flight.mostAtOnce(); peak != noteSlots {
		t.Errorf("%d note lookups were in progress at once, want %d", peak, noteSlots)
	}
	// No transcript: which pick's lookup takes a freed slot first is the
	// scheduler's choice, so the order of the reads is not repeatable.
}

// A pick whose lookups the round's budget cut short is remembered as a miss
// for ten minutes, so viewing the page again does not search again, and
// searched once more after that.
func TestCalendarTimedOutNotesWaitBeforeRetrying(t *testing.T) {
	h := start(t, scenario{noteGrace: mondayNoteGrace, progress: followIdle, routes: map[string]route{searchPath: ok("empty.json")}})
	held := h.mp.holdCalls(searchPath, heldSearches)
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "新番放送")
	filled := h.tg.expect("edit:1")
	select {
	case <-filled:
	case <-time.After(2 * actionTimeout):
		t.Fatalf("the round never ended\n%s", h.tr)
	}
	next := firstUnentered(t, held)
	searched := strings.Count(h.tr.String(), searchLine)
	h.say(alice, alice, "/trending")
	h.tap(alice, 2, "新番放送")
	h.shows(2, "星期一（今天）")
	if got := strings.Count(h.tr.String(), searchLine); got != searched {
		t.Errorf("viewing the page again searched %d more times", got-searched)
	}
	h.advance(timeoutMissTTL)
	h.say(alice, alice, "/trending")
	h.tap(alice, 3, "新番放送")
	h.wait(next.entered, "a timed out pick searched again")
	for _, call := range held {
		call.allow()
	}
	// No transcript: which lookups take the few slots before the budget
	// ends is the scheduler's choice, so the reads are not repeatable.
}

// firstUnentered is the first held call no request has reached yet, which the
// next search to arrive will take.
func firstUnentered(t *testing.T, calls []*heldCall) *heldCall {
	t.Helper()
	for _, call := range calls {
		select {
		case <-call.entered:
		default:
			return call
		}
	}
	t.Fatal("every held search was reached")
	return nil
}

// A calendar pick whose card details are slow keeps the card to the shorter
// budget, as the calendar page already gave it a synopsis.
func TestCalendarCardDoesNotWaitOnSlowDetails(t *testing.T) {
	h := start(t, scenario{
		routes: map[string]route{
			searchPath:                      ok("empty.json"),
			seasonsPath:                     ok("seasons_aonohako.json"),
			"GET /api/v1/media/" + aonohako: {status: 200, fixture: "detail_aonohako.json", delay: slowDetails},
			"GET /api/v1/subscribe/media/" + aonohako: ok("subscription_none.json"),
		},
		searches: map[string]string{"青之箱 第二季": "search_calendar_aonohako.json"},
	})
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "新番放送")
	h.tap(alice, 1, "日")
	began := time.Now()
	h.tap(alice, 1, "2")
	h.shows(1, "每天早晨在晨练时")
	if elapsed := time.Since(began); elapsed >= calendarCardWait {
		t.Errorf("the card waited %s on slow details", elapsed)
	}
	h.tr.verify(t)
}
