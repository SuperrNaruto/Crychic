package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// A lost response after display must not create a second message. The next
// command is an input-lane barrier: the first send has finished handling.
func TestLostSendResponseDoesNotDuplicate(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{searchPath: ok("search_dune.json")}})
	lost := h.tg.loseNextResponse("sendRichMessage")
	h.say(alice, alice, "/search 沙丘")
	h.wait(lost, "the lost response after displaying search results")
	h.say(alice, alice, "/help")
	const searchAndHelp = 2
	if got := strings.Count(h.tr.String(), "<< sendRichMessage chat="); got != searchAndHelp {
		t.Errorf("search and help displayed %d messages, want %d", got, searchAndHelp)
	}
	h.shows(1, "帮你找到这些")
	h.shows(2, "我能帮你做这些")
	h.tr.verify(t)
}

func receiptScenario(t *testing.T) scenario {
	t.Helper()
	const mediaID = "42"
	media := map[string]any{
		"media_source": "themoviedb", "media_id": mediaID, "type": "电影", "title": "无海报影片",
	}
	return scenario{routes: map[string]route{
		searchPath:                               ok(writeFixture(t, []map[string]any{media})),
		"GET /api/v1/media/" + mediaID:           ok(writeFixture(t, media)),
		"GET /api/v1/subscribe/media/" + mediaID: ok("subscription_none.json"),
		subscribePath:                            ok("subscribe_created.json"),
	}}
}

// A successful write whose receipt failed is recovered without another write,
// automatically or by pressing the confirmation that is still visible.
func TestSuccessfulReceiptDeliveryRecovers(t *testing.T) {
	for _, mode := range []string{"automatic", "button"} {
		t.Run(mode, func(t *testing.T) {
			h := start(t, receiptScenario(t))
			h.say(alice, alice, "无海报影片")
			failed := h.tg.failNextEdit(1)
			recovered := h.tg.expect("edit:1")
			h.tap(alice, 1, "确认订阅")
			h.wait(failed, "the rejected successful receipt")
			if mode == "button" {
				h.tap(alice, 1, "确认订阅")
			}
			h.wait(recovered, "the recovered successful receipt")
			h.shows(1, "帮你订好《无海报影片》")
			const oneWrite = 1
			if got := strings.Count(h.tr.String(), "-> MoviePilot "+subscribePath); got != oneWrite {
				t.Errorf("receipt recovery made %d subscription writes, want %d", got, oneWrite)
			}
			h.tr.verify(t)
		})
	}
}

// A rate-limited edit neither clears the poster cache nor retries before
// retry_after. An impatient owner and another user cannot bypass the delay.
func TestReceiptRetryHonorsRateLimit(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"),
		duneLookup: ok("subscription_none.json"), subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	const retrySeconds = 1
	limited := h.tg.rejectNextEdit(1, retrySeconds)
	recovered := h.tg.expect("edit:1")
	confirm, _ := findButton(mustMessage(h, 1).rows, "确认订阅")
	began := time.Now()
	h.tap(alice, 1, "确认订阅")
	h.wait(limited, "the Telegram flood-control response")
	h.shows(1, "要订阅")
	h.tapData(bob, 1, confirm)
	h.tapData(alice, 1, confirm)
	h.wait(recovered, "the rate-limited successful receipt")
	if time.Since(began) < retrySeconds*time.Second {
		t.Error("receipt retried before Telegram's retry_after")
	}
	h.shows(1, "帮你订好《沙丘》")
	photos := mustMessage(h, 1).media
	if len(photos) == 0 || !photos[0].reused {
		t.Error("rate limiting discarded the valid poster file_id")
	}
	h.tr.verify(t)
}

// A late initial response cannot attach its old follower after the owner has
// cancelled. Observe several real refresh intervals, not the injected clock.
func TestLateInitialResponsePreservesCancellation(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath:  ok(selectedFixture(t, "search_dune.json", 0)),
		duneDetails: ok("detail_dune.json"), duneLookup: ok("subscription_none.json"),
		duneTorrents: {status: http.StatusOK, fixture: "torrents_dune.json", delay: time.Minute},
	}})
	release := h.tg.holdNextResponse("sendRichMessage")
	defer release()
	h.say(alice, alice, "下载 沙丘")
	h.tap(alice, 1, "取消")
	h.shows(1, "已经取消啦")
	unwanted := h.tg.expect("edit:1")
	release()
	h.say(alice, alice, "/help")
	const refreshWindows = 3
	select {
	case <-unwanted:
		t.Error("the late initial response overwrote the cancellation")
	case <-time.After(refreshWindows * h.cfg.FollowEvery):
	}
	h.shows(1, "已经取消啦")
	h.tr.verify(t)
}

// Revoking a private requester stops their notices, not the shared request's
// notices to another still-authorized requester.
func TestRevokedRequesterStopsReceivingNotices(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{
		searchPath: ok("search_dune.json"), duneDetails: ok("detail_dune.json"),
		duneLookup: ok("subscription_none.json"), subscribePath: ok("subscribe_created.json"),
	}})
	h.say(alice, alice, "/search 沙丘")
	h.tap(alice, 1, duneMovie)
	h.tap(alice, 1, "确认订阅")
	h.say(bob, bob, "/search 沙丘")
	h.tap(bob, 2, duneMovie)
	h.tap(bob, 2, "确认订阅")
	h.cfg.TelegramAllowed = []int64{bob}
	h.restart()
	h.say(alice, alice, "/start")
	h.shows(3, "没有权限")
	h.arrives(bob, duneFile())
	h.transfers()
	const bobNotice = 4
	noticeAt(h, arrivalCheck{message: bobNotice, chat: bob, title: "《沙丘》"})
	if _, extra := h.tg.message(bobNotice + 1); extra {
		t.Error("the revoked requester also received an arrival")
	}
	h.tr.verify(t)
}

// noticeAt checks the observable recipient and content together: the right
// text delivered to another chat is still a lost notice.
type arrivalCheck struct {
	message int
	chat    int64
	title   string
}

func noticeAt(h *harness, want arrivalCheck) {
	h.t.Helper()
	msg, ok := h.tg.message(want.message)
	if !ok || msg.chat != want.chat || !strings.Contains(msg.text, want.title) || !strings.Contains(msg.text, "MoviePilot 通知") {
		h.t.Errorf("arrival %d: want %q in chat %d, got chat %d text %q", want.message, want.title, want.chat, msg.chat, msg.text)
	}
}
