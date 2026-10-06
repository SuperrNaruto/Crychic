package e2e

import (
	"encoding/json"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// Larger real-shaped lists must remain accessible, including the last item,
// without ever exceeding Telegram's text limit.
func TestSubscriptionListPagesWithinTelegramLimit(t *testing.T) {
	const subscriptions = 150
	h := start(t, scenario{routes: map[string]route{
		subsPath: ok(manySubscriptions(t, subscriptions)),
	}})
	h.say(alice, alice, "/subs")
	for {
		msg := mustMessage(h, 1)
		checkTextBudget(t, msg.text)
		if _, more := findButton(msg.rows, "下一页 ›"); !more {
			break
		}
		h.tap(alice, 1, "下一页 ›")
	}
	h.shows(1, `<li value="150">`)
	h.tap(alice, 1, "‹ 上一页")
	h.tr.verify(t)
}

func manySubscriptions(t *testing.T, count int) string {
	t.Helper()
	var subs []map[string]any
	for i := range count {
		subs = append(subs, map[string]any{"id": i + 1, "name": "绝命毒师", "type": "电视剧", "season": 2, "state": "R", "lack_episode": 13, "total_episode": 13})
	}
	return writeFixture(t, subs)
}

func checkTextBudget(t *testing.T, text string) {
	t.Helper()
	const telegramLimit = 4096
	plain := html.UnescapeString(regexp.MustCompile(`<[^>]*>`).ReplaceAllString(text, ""))
	if size := utf8.RuneCountInString(plain); size > telegramLimit {
		t.Errorf("message has %d characters, limit %d", size, telegramLimit)
	}
}

// Independent metadata calls should overlap, without changing subscription
// safety checks or making any write before confirmation.
func TestSlowMetadataDoesNotAddEveryDelay(t *testing.T) {
	const callDelay = 200 * time.Millisecond
	const replyBudget = 3 * callDelay
	h := start(t, scenario{routes: map[string]route{
		searchPath:       ok("search_dune.json"),
		duneDetails:      {status: http.StatusOK, fixture: "detail_dune.json", delay: callDelay},
		libraryMoviePath: {status: http.StatusOK, fixture: "library_movie_missing.json", delay: callDelay},
		downloadsPath:    {status: http.StatusOK, fixture: "downloads_none.json", delay: callDelay},
		duneLookup:       {status: http.StatusOK, fixture: "subscription_none.json", delay: callDelay},
	}})
	h.say(alice, alice, "/search 沙丘")
	start := time.Now()
	h.tap(alice, 1, duneMovie)
	elapsed := time.Since(start)
	t.Logf("card ready after %s (each backend delay %s)", elapsed, callDelay)
	if elapsed >= replyBudget {
		t.Errorf("independent reads exceeded %s", replyBudget)
	}
	if !strings.Contains(mustMessage(h, 1).text, "确认订阅") {
		t.Error("card did not reach confirmation")
	}
	h.tr.verify(t)
}

// One unresponsive Telegram send must time out without requiring shutdown;
// later commands are still answered by the same running app.
func TestTelegramSendHasDeadline(t *testing.T) {
	const cancellationBudget = 15 * time.Second
	h := start(t, scenario{})
	stalled := h.tg.stallNext("sendRichMessage")
	h.chatter(alice, alice, "/start")
	select {
	case <-stalled:
	case <-time.After(cancellationBudget):
		t.Fatal("Telegram send did not time out")
	}
	h.say(alice, alice, "/start")
	h.shows(1, "Crychic")
	h.tr.verify(t)
}

// Polling an unchanged server must not replace the durable state file.
func TestIdlePollingDoesNotRewriteState(t *testing.T) {
	h := start(t, scenario{})
	h.transfers()
	path := filepath.Join(h.cfg.DataDir, "requests.json")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	h.transfers()
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Error("idle polling replaced the unchanged state file")
	}
	h.tr.add(">> unchanged server polled twice; durable state file retained")
	h.tr.verify(t)
}

// A large transfer is summarized in one live message rather than rejected
// for its length; aggregate progress still counts every file.
func TestLargeTransferFitsTelegramLimit(t *testing.T) {
	const fileCount = 500
	h := start(t, scenario{routes: taskRoutes()})
	var tasks []map[string]any
	for i := range fileCount {
		tasks = append(tasks, map[string]any{"state": "waiting", "meta": map[string]any{"begin_episode": i + 1}})
	}
	path := writeFixture(t, []map[string]any{{"media": map[string]any{"media_source": "themoviedb", "media_id": "274580", "title": "颂乐人偶"}, "season": 1, "tasks": tasks}})
	h.mp.setRoute(queuePath, ok(path))
	h.say(alice, alice, "/tasks")
	h.tap(alice, 1, mujicaTask)
	h.shows(1, "0/500")
	checkTextBudget(t, mustMessage(h, 1).text)
	h.tap(alice, 1, "停止刷新")
	h.tr.verify(t)
}

func writeFixture(t *testing.T, data any) string {
	t.Helper()
	const fixtureMode = 0o600
	path := filepath.Join(t.TempDir(), "response.json")
	raw, err := json.Marshal(map[string]any{"success": true, "data": data})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, fixtureMode); err != nil {
		t.Fatal(err)
	}
	base, err := filepath.Abs("testdata/moviepilot")
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(base, path)
	if err != nil {
		t.Fatal(err)
	}
	return relative
}
