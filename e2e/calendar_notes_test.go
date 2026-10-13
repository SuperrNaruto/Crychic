package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
)

var mondayPicks = []string{"390200", "418229", "511936", "614640", "624923"}

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
	var held []*heldCall
	for _, id := range mondayPicks {
		held = append(held, h.mp.holdCalls(bangumiInfoPath+id, 1)...)
	}
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "新番放送")
	h.shows(1, "简介补充中")
	if strings.Contains(mustMessage(h, 1).text, "MoviePilot 返回的女娲简介") {
		t.Fatal("a synopsis still being looked up was shown")
	}
	buttons := buttonLines(mustMessage(h, 1).rows)
	filled := h.tg.expect("edit:1")
	for _, call := range held {
		h.wait(call.entered, "the held synopsis lookup")
		call.allow()
	}
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

// Opening the calendar fills the notes of the whole week in the background,
// so a weekday switched to later is shown without a lookup.
func TestCalendarWarmsTheWeek(t *testing.T) {
	h := start(t, scenario{calendarWarm: true, routes: map[string]route{searchPath: ok("empty.json")}})
	warmed, picks := otherDays(t)
	h.tr.hideReads(warmed)
	h.say(alice, alice, "/trending")
	h.tap(alice, 1, "新番放送")
	h.waitReads(subjectLine, picks)
	hidden := len(h.tr.hiddenLines)
	h.tap(alice, 1, "二")
	h.shows(1, "星期二</h3>")
	h.tap(alice, 1, "日")
	h.shows(1, "星期日</h3>")
	if strings.Contains(mustMessage(h, 1).text, "简介补充中") {
		t.Error("a warmed day still looked up its synopses")
	}
	if got := len(h.tr.hiddenLines); got != hidden {
		t.Errorf("switching weekday made %d more requests", got-hidden)
	}
	h.tr.verify(t)
}

// otherDays tells the requests about the calendar's picks of other days than
// Monday's, and how many picks they are.
func otherDays(t *testing.T) (func(line string) bool, int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "bangumi", calendarFixture))
	if err != nil {
		t.Fatal(err)
	}
	var days []struct {
		Items []struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			NameCN  string `json:"name_cn"`
			Weekday int    `json:"air_weekday"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &days); err != nil {
		t.Fatal(err)
	}
	var parts []string
	picks := 0
	for _, day := range days {
		for _, it := range day.Items {
			if it.Weekday == 1 {
				continue
			}
			picks++
			id := strconv.Itoa(it.ID)
			parts = append(parts, subjectsPath+id, bangumiRoute+id)
			for _, name := range []string{it.NameCN, it.Name} {
				if name != "" {
					parts = append(parts, "title="+name+"&type=media")
				}
			}
		}
	}
	return func(line string) bool {
		return strings.HasPrefix(line, "-> ") && slices.ContainsFunc(parts, func(p string) bool { return strings.HasSuffix(line, p) })
	}, picks
}

// waitReads waits until the transcript has hidden want reads containing part.
func (h *harness) waitReads(part string, want int) {
	h.t.Helper()
	deadline := time.After(actionTimeout)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for h.tr.hiddenReads(part) < want {
		select {
		case <-tick.C:
		case <-deadline:
			h.t.Fatalf("only %d of %d background reads arrived", h.tr.hiddenReads(part), want)
		}
	}
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
