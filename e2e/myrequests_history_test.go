package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Completion is an arrival fact, not proof of playback availability. The
// actual notification transition must write history before its watch ends.
func TestMyRequestsCompletedSurvivesRestart(t *testing.T) {
	h := start(t, scenario{routes: movieRequestRoutes(), lagging: true, libraryWait: "0s"})
	requestMovieDownload(h)
	before := h.tr.String()
	h.say(alice, alice, "/requests")
	file := duneFile()
	file.Hash, file.Src = addedHash, "/private/"+requestPrivate+"/movie.mkv"
	h.arrives(alice, file)
	h.tap(alice, 2, "最近完成")
	h.tap(alice, 2, "2")
	h.shows(2, "完成记录")
	h.shows(2, "未入库")
	h.shows(2, "完成记录不代表媒体库当前可见")
	h.restart()
	h.tap(alice, 2, "刷新")
	h.shows(2, "/requests")
	h.say(alice, alice, "/requests")
	h.tap(alice, 4, "最近完成")
	h.tap(alice, 4, "1")
	h.shows(4, "未入库")
	h.mp.catchUp()
	h.tap(alice, 4, "刷新")
	h.shows(4, "已入库")
	h.say(bob, bob, "/requests")
	h.shows(5, "还没有通过我登记的请求")
	h.transfers()
	noticeCount(h, 1)
	assertRequestStatePrivate(h)
	assertRequestReadsOnly(h, before)
	h.tr.verify(t)
}

// A legacy request with some arrivals is not a fabricated completed
// request. Old cursor/pending fields survive adding the bounded history.
func TestMyRequestsLegacyAndHistoryRetention(t *testing.T) {
	h := start(t, scenario{routes: myRequestRoutes(t)})
	requestBreakingSeason(h)
	h.arrives(group, breakingBad.file("S02", "E01"))
	h.say(alice, alice, "/requests")
	h.tap(alice, 3, "最近完成")
	h.shows(3, "还没有完成记录")
	h.arrives(group, breakingBad.file("S02", "E02-E13"))
	seedPriorCompletedRequests(h)
	h.say(alice, alice, "/requests")
	h.tap(alice, 5, "最近完成")
	h.shows(5, "过往请求49")
	h.tap(alice, 5, "下一页 ›")
	h.shows(5, "过往请求29")
	h.tap(alice, 5, "22")
	h.shows(5, "完成记录")
	h.shows(5, "从第 1 集开始")
	h.tap(alice, 5, "返回")
	h.shows(5, "过往请求29")
	h.say(alice, alice, "/search 绝命毒师")
	h.tap(alice, 6, "1")
	h.tap(alice, 6, "第 3 季")
	h.tap(alice, 6, "从第 1 集开始")
	h.arrives(alice, breakingBad.file("S03", "E01-E13"))
	h.tap(alice, 5, "刷新")
	h.shows(5, "《绝命毒师》第 3 季")
	assertCompletedRetention(h)
	assertRequestStatePrivate(h)
	h.tr.verify(t)
}

// A known custom episode group must survive row closure and restart as
// unmapped. Default season numbers cannot fabricate a completed request.
func TestMyRequestsUnmappedCompletionIsNotInvented(t *testing.T) {
	h := start(t, scenario{routes: myRequestRoutes(t)})
	requestBreakingSeason(h)
	sub := breakingSeason(1, 2)
	sub["episode_group"] = "alternate-layout"
	setNotifySubscription(h, sub)
	h.arrives(group, breakingBad.file("S02", "E01"))
	h.say(alice, alice, "/requests")
	h.tap(alice, 3, "1")
	h.shows(3, "剧集组映射未确认")
	h.mp.setRoute(subsPath, ok("subscriptions_none.json"))
	h.mp.setRoute(notifySubscription, ok("subscription_none.json"))
	h.restart()
	h.arrives(group, breakingBad.file("S02", "E02-E13"))
	if strings.Contains(mustMessage(h, 4).text, "全部到齐") {
		t.Error("unmapped episode groups were announced as a completed default season")
	}
	h.restart()
	h.say(alice, alice, "/requests")
	h.tap(alice, 5, "最近完成")
	h.shows(5, "还没有完成记录")
	h.tap(alice, 5, "跟踪中")
	h.tap(alice, 5, "1")
	h.shows(5, "剧集组映射未确认")
	h.shows(5, "<td>未知</td>")
	assertRequestStatePrivate(h)
	h.tr.verify(t)
}

// Seed only prior, already-completed storage records, cloned from a real
// completion above. A subsequent real arrival owns pruning and persistence.
func seedPriorCompletedRequests(h *harness) {
	h.t.Helper()
	h.stop()
	h.stop = func() {}
	path := filepath.Join(h.cfg.DataDir, "requests.json")
	st := requestState(h)
	var entries []map[string]any
	if err := json.Unmarshal(st["completed_requests"], &entries); err != nil || len(entries) != 1 {
		h.t.Fatal("the actual arrival did not persist one completed request")
	}
	const priorCompletedCount = 50
	var prior []map[string]any
	for i := range priorCompletedCount {
		var record map[string]any
		raw, _ := json.Marshal(entries[0])
		_ = json.Unmarshal(raw, &record)
		record["key"], record["title"] = fmt.Sprintf("prior-%d", i), fmt.Sprintf("过往请求%d", i)
		prior = append(prior, record)
	}
	st["completed_requests"], _ = json.Marshal(prior)
	raw, err := json.Marshal(st)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, legacyStateMode); err != nil {
		h.t.Fatal(err)
	}
	h.tr.add(">> Crychic restarts with 50 prior completion records; the transfer cursor is unchanged")
	h.launch()
}

func requestState(h *harness) map[string]json.RawMessage {
	h.t.Helper()
	raw, err := os.ReadFile(filepath.Join(h.cfg.DataDir, "requests.json"))
	if err != nil {
		h.t.Fatal(err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(raw, &state); err != nil {
		h.t.Fatal(err)
	}
	return state
}

func assertCompletedRetention(h *harness) {
	h.t.Helper()
	st := requestState(h)
	var entries []map[string]any
	if err := json.Unmarshal(st["completed_requests"], &entries); err != nil {
		h.t.Fatal(err)
	}
	const keptCompleted = 50
	if len(entries) != keptCompleted {
		h.t.Error("completed request retention did not replace the oldest record")
	}
	for _, entry := range entries {
		if entry["title"] == "过往请求0" {
			h.t.Error("the oldest completed record survived pruning")
		}
	}
	if len(st["transfer_cursor"]) == 0 {
		h.t.Error("completion history discarded the arrival cursor")
	}
}

func assertRequestStatePrivate(h *harness) {
	h.t.Helper()
	st := requestState(h)
	raw, err := json.Marshal(st)
	if err != nil {
		h.t.Fatal(err)
	}
	if strings.Contains(string(raw), requestPrivate) {
		h.t.Error("private diagnostics or server paths reached persisted request state")
	}
	for _, field := range []string{"Name", "Address", "errmsg", "recovery_action", "src"} {
		if strings.Contains(string(st["completed_requests"]), `"`+field+`"`) {
			h.t.Errorf("completion history unnecessarily retained %s", field)
		}
	}
}
