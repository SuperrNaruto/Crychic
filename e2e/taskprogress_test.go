package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	mujicaID    = "274580"
	mujicaE13   = "<td>E13</td><td>▶️ 整理中</td>"
	mujicaE13At = "<td>E13</td><td>▶️ 整理中 38%</td>"
)

// mujicaProgress is the plugin's record of a running 颂乐人偶 S01 file, the
// way the queue names it: no download hash or instance for this fixture.
func mujicaProgress(mediaID string, episode int, active bool, percent float64) map[string]any {
	return map[string]any{
		"media_source": "themoviedb", "media_id": mediaID, "season": 1,
		"begin_episode": episode, "end_episode": nil, "download_hash": nil, "downloader": nil,
		"active": active, "percent": percent,
	}
}

// progresses makes the Crychic plugin report files from now on and waits
// for the live view in message msgID to show the change.
func (h *harness) progresses(msgID int, files ...map[string]any) {
	h.t.Helper()
	raw, _ := json.Marshal(files)
	h.tr.add(fmt.Sprintf(">> Crychic plugin now reports transfer progress %s", raw))
	done := h.tg.expect(fmt.Sprintf("edit:%d", msgID))
	h.mp.native.setProgress(files)
	h.wait(done, "a live view refresh")
}

// A followed transfer shows a running file's byte progress once the plugin
// reports it for that file; a record of another title stays out, and a file
// that is no longer running shows its state only. Without the plugin route
// the view keeps the file states.
func TestTransferShowsFileProgress(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{queuePath: ok("queue_mujica.json")}})
	h.say(alice, alice, "/tasks")
	h.tap(alice, 1, "1")
	h.shows(1, mujicaE13)
	h.progresses(1, mujicaProgress("999999", 13, true, 90), mujicaProgress(mujicaID, 13, true, 37.5))
	h.shows(1, mujicaE13At)
	h.reports(1, queuePath, "queue_mujica_later.json")
	h.shows(1, "<td>E12</td><td>▶️ 整理中</td>")
	if strings.Contains(mustMessage(h, 1).text, "%") {
		t.Error("a finished or unrecorded file shows byte progress")
	}
	h.tr.verify(t)
}

// Progress shows only for the one running file a record can belong to:
// two running files with the same identity, or a record of a finished or
// abandoned move, leave the states as they are.
func TestTransferProgressNeedsOneOwner(t *testing.T) {
	h := start(t, scenario{routes: map[string]route{queuePath: ok(twinRunningFiles(t))}})
	h.mp.native.setProgress([]map[string]any{mujicaProgress(mujicaID, 13, true, 50)})
	h.say(alice, alice, "/tasks")
	h.tap(alice, 1, "1")
	if text := mustMessage(h, 1).text; strings.Count(text, mujicaE13) != 2 || strings.Contains(text, "50%") {
		t.Errorf("twin running files got a record's progress:\n%s", text)
	}
	h.tap(alice, 1, "返回")
	h.mp.setRoute(queuePath, ok("queue_mujica.json"))
	h.mp.native.setProgress([]map[string]any{mujicaProgress(mujicaID, 13, false, 100)})
	h.tap(alice, 1, "1")
	h.shows(1, mujicaE13)
	if strings.Contains(mustMessage(h, 1).text, "100%") {
		t.Error("an ended progress record shows as a running file's progress")
	}
	h.tr.verify(t)
}

// twinRunningFiles is the 颂乐人偶 queue with E13 running twice, as two
// versions of one episode would be.
func twinRunningFiles(t *testing.T) string {
	t.Helper()
	return queueWith(t, func(tasks []any) []any {
		return append(tasks[:1], tasks[0])
	})
}

// queueWith is the 颂乐人偶 transfer queue fixture after edit changed the
// tasks of its one job.
func queueWith(t *testing.T, edit func([]any) []any) string {
	t.Helper()
	var env struct{ Data []map[string]any }
	raw, err := os.ReadFile(filepath.Join("testdata", "moviepilot", "queue_mujica.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	env.Data[0]["tasks"] = edit(env.Data[0]["tasks"].([]any))
	return writeFixture(t, env.Data)
}
