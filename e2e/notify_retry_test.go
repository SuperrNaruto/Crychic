package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	sameSecondBatch    = 51
	firstBatchEpisode  = 2
	lastMygoEpisode    = 13
	transferSecond     = "2026-10-05 12:01:00"
	nextTransferSecond = "2026-10-05 12:01:01"
	legacyStateMode    = 0o600
	settleArrivals     = 2 * time.Minute
)

func datedTransfer(file transfer, date string) transfer {
	file.Date = date
	return file
}

func mygoSubscriptionRoutes() map[string]route {
	return map[string]route{
		searchPath: ok("search_mygo.json"), mygoDetails: ok("detail_mygo.json"),
		seasonsPath: ok("seasons_mygo.json"), mygoLookup: ok("subscription_none.json"),
		subscribePath: ok("subscribe_created.json"),
	}
}

// batchTransfers is a normal batch of 51 files completed in one second:
// MyGO E02-E12 plus Conan files, with MyGO E02 on the second history page.
func batchTransfers() []transfer {
	var files []transfer
	for ep := firstBatchEpisode; ep < lastMygoEpisode; ep++ {
		files = append(files, datedTransfer(mygo.file("S01", fmt.Sprintf("E%02d", ep)), transferSecond))
	}
	for ep := 1; len(files) < sameSecondBatch; ep++ {
		files = append(files, datedTransfer(conan.file("S01", fmt.Sprintf("E%02d", ep)), transferSecond))
	}
	return files
}

func requestMygo(h *harness) {
	h.t.Helper()
	const request = 1
	h.say(alice, alice, "/search 迷途之子")
	h.tap(alice, request, "1")
	h.tap(alice, request, "第 1 季")
	h.tap(alice, request, "从第 1 集开始")
}

func noticeCount(h *harness, want int) {
	h.t.Helper()
	if got := strings.Count(h.tr.String(), "<h3>📥 入库啦</h3>"); got != want {
		h.t.Errorf("arrival messages = %d, want %d", got, want)
	}
}

// A retry can enter the second page with an old id and the same completion
// second. Restarting must keep old successes quiet, but not lose the retry;
// the last new episode then completes the season.
func TestRetriedTransferAfterRestartIsAnnounced(t *testing.T) {
	const firstNotice, retryNotice, completeNotice = 2, 3, 4
	const wantedNotices = 3
	h := start(t, scenario{quiet: "0s", routes: mygoSubscriptionRoutes()})
	requestMygo(h)
	failed := h.mp.failedTransfer(datedTransfer(mygo.file("S01", "E01"), transferSecond))
	h.tr.add(fmt.Sprintf(">> MoviePilot records failed E01 as id=%d", failed))
	h.arrives(alice, batchTransfers()...)
	h.shows(firstNotice, "E02–E12 到家啦")
	h.transfers()
	h.restart()
	h.transfers()
	h.tr.add(fmt.Sprintf(">> MoviePilot retries id=%d successfully in the same second, on page two", failed))
	h.mp.retryTransfer(failed, transferSecond)
	h.transfers()
	noticeAt(h, arrivalCheck{message: retryNotice, chat: alice, title: "E01 到家啦"})
	h.arrives(alice, datedTransfer(mygo.file("S01", "E13"), nextTransferSecond))
	noticeAt(h, arrivalCheck{message: completeNotice, chat: alice, title: "这一季你要的剧集全部到齐啦"})
	h.transfers()
	noticeCount(h, wantedNotices)
	h.tr.verify(t)
}

// The baseline includes every already successful row of its newest second,
// even across pages. A later success in that same second still belongs in
// the digest, while none of the pre-start files do.
func TestSameSecondBaselineExcludesOldHistory(t *testing.T) {
	const digestMessage = 1
	h := start(t, scenario{history: batchTransfers(), digest: "mon 20:00"})
	h.tr.add(fmt.Sprintf(">> Crychic starts after %d successful files in one second", sameSecondBatch))
	h.transfers()
	h.transfers(datedTransfer(duneFile(), transferSecond))
	h.digestArrives(untilDigest)
	h.shows(digestMessage, "这周有 1 部")
	h.shows(digestMessage, "《沙丘》")
	text := mustMessage(h, digestMessage).text
	if strings.Contains(text, mygo.title) || strings.Contains(text, conan.title) {
		t.Error("the first digest replays files that predate the bot")
	}
	h.transfers()
	h.tr.verify(t)
}

// legacyRestart recreates the previously shipped storage format at the
// persisted JSON boundary. The app produced all pending/digest data itself;
// only the new cursor field is removed while stopped.
func legacyRestart(h *harness, offline transfer) {
	h.t.Helper()
	h.tr.add(">> Crychic stops; requests.json keeps the legacy last_transfer checkpoint")
	h.stop()
	h.stop = func() {}
	path := filepath.Join(h.cfg.DataDir, "requests.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		h.t.Fatal(err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(raw, &state); err != nil {
		h.t.Fatal(err)
	}
	delete(state, "transfer_cursor")
	raw, err = json.MarshalIndent(state, "", "  ")
	if err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, legacyStateMode); err != nil {
		h.t.Fatal(err)
	}
	h.logTransfers([]transfer{offline})
	h.mp.add(offline)
	h.tr.add(">> Crychic upgrades and resumes from the legacy checkpoint")
	h.launch()
}

// Upgrading retains a pending arrival and digest, catches new ids added
// while stopped without replaying baseline history, and starts noticing
// old failed ids that succeed after migration.
func TestLegacyArrivalStateKeepsPendingAndDigest(t *testing.T) {
	const combinedNotice, digestMessage, retryNotice = 2, 3, 5
	const wantedNotices = 2
	h := start(t, scenario{
		history: []transfer{duneFile()}, digest: "mon 20:00", quiet: "1m",
		routes: mygoSubscriptionRoutes(),
	})
	requestMygo(h)
	failed := h.mp.failedTransfer(datedTransfer(mygo.file("S01", "E01"), transferSecond))
	h.tr.add(fmt.Sprintf(">> MoviePilot records failed E01 as id=%d", failed))
	h.transfers(datedTransfer(mygo.file("S01", "E02"), transferSecond))
	legacyRestart(h, datedTransfer(mygo.file("S01", "E03"), nextTransferSecond))
	h.transfers()
	told := h.tg.expect(fmt.Sprintf("send:%d", alice))
	h.advance(settleArrivals)
	h.wait(told, "the pending and offline arrivals after upgrade")
	h.shows(combinedNotice, "E02–E03 到家啦")
	h.digestArrives(untilDigest)
	h.shows(digestMessage, "第 1 季 · E02–E03")
	if strings.Contains(mustMessage(h, digestMessage).text, "《沙丘》") {
		t.Error("migration replays a transfer from the old baseline into the digest")
	}
	h.tr.add(fmt.Sprintf(">> MoviePilot retries id=%d successfully after migration", failed))
	h.mp.retryTransfer(failed, nextTransferSecond)
	h.transfers()
	h.advance(settleArrivals)
	h.transfers()
	noticeAt(h, arrivalCheck{message: retryNotice, chat: alice, title: "E01 到家啦"})
	h.restart()
	h.transfers()
	noticeCount(h, wantedNotices)
	h.tr.verify(t)
}
