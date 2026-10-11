package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if got := strings.Count(h.tr.String(), "<h3>MoviePilot 通知</h3>"); got != want {
		h.t.Errorf("arrival messages = %d, want %d", got, want)
	}
}

// A retry can enter the second page with an old id and the same completion
// second. Restarting must keep old successes quiet, but not lose the retry;
// the last new episode then completes the season.

// The baseline includes every already successful row of its newest second,
// even across pages. A later success in that same second still belongs in
// the digest, while none of the pre-start files do.

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
