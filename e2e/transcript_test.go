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
	for _, b := range body {
		for _, line := range strings.Split(b, "\n") {
			tr.lines = append(tr.lines, "   "+line)
		}
	}
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
