package e2e

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden transcripts")

// transcript is the ordered record of everything crossing the bot's
// boundaries: user actions, MoviePilot calls and Telegram calls. Each
// scenario's transcript is committed under testdata/transcripts and is the
// reviewable, repeatable artifact of the run.
type transcript struct {
	mu    sync.Mutex
	lines []string
}

func (tr *transcript) add(head string, body ...string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.lines = append(tr.lines, head)
	if len(body) == 0 {
		tr.settle()
	}
	for _, b := range body {
		for _, line := range strings.Split(b, "\n") {
			tr.lines = append(tr.lines, "   "+line)
		}
	}
}

// settle sorts the last line into the run of reads of the same route
// before it: the bot makes such reads at once (a chart page's lookups), so
// they arrive in any order. Media searches and details count as one route,
// since a calendar page looks up both at once.
func (tr *transcript) settle() {
	for i := len(tr.lines) - 1; i > 0; i-- {
		prev, last := tr.lines[i-1], tr.lines[i]
		if readRoute(prev) == "" || readRoute(prev) != readRoute(last) || readOrder(prev) <= readOrder(last) {
			return
		}
		tr.lines[i-1], tr.lines[i] = last, prev
	}
}

// readRoute is a MoviePilot read's path without its query and trailing id,
// or "" for any other line.
// readOrder sorts media searches before details, as a pick made one at a
// time reads them.
func readOrder(line string) string {
	return strings.Replace(line, mediaRoute+"search", mediaRoute+" search", 1)
}

// mediaRoute is where media searches and details are read.
const mediaRoute = "/api/v1/media/"

func readRoute(line string) string {
	path, found := strings.CutPrefix(line, "-> MoviePilot GET ")
	if !found {
		return ""
	}
	path, _, _ = strings.Cut(path, "?")
	if path == mediaRoute+"search" {
		return mediaRoute
	}
	return strings.TrimRight(path, "0123456789")
}

func (tr *transcript) String() string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return strings.Join(tr.lines, "\n") + "\n"
}

// verify compares the transcript with the scenario's golden file.
func (tr *transcript) verify(t *testing.T) {
	t.Helper()
	path := filepath.Join("testdata", "transcripts", t.Name()+".txt")
	got := tr.String()
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)\n--- got ---\n%s", err, got)
	}
	if got != string(want) {
		t.Fatalf("transcript differs from %s (run with -update after reviewing)\n--- got ---\n%s\n--- want ---\n%s",
			path, got, want)
	}
}

func buttonLines(rows [][]button) string {
	var b strings.Builder
	for i, row := range rows {
		if i > 0 {
			b.WriteString("\n")
		}
		for j, btn := range row {
			if j > 0 {
				b.WriteString(" ")
			}
			fmt.Fprintf(&b, "[%s | %s]", btn.Text, btn.CallbackData)
		}
	}
	return b.String()
}
