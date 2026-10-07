package e2e

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	mu       sync.Mutex
	lines    []string
	sessions map[string]string
}

func (tr *transcript) add(head string, body ...string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.lines = append(tr.lines, tr.normalize(head))
	if len(body) == 0 {
		tr.settle()
	}
	for _, b := range body {
		for _, line := range strings.Split(b, "\n") {
			tr.lines = append(tr.lines, strings.TrimRight("   "+tr.normalize(line), " \t"))
		}
	}
}

var callbackToken = regexp.MustCompile(`\b[0-9]+:[a-z]+:-?[0-9]+\b`)

// Session identities are random in production. Normalize only the transcript,
// never the updates sent to the app, preserving identity across restarts.
func (tr *transcript) normalize(text string) string {
	if tr.sessions == nil {
		tr.sessions = map[string]string{}
	}
	return callbackToken.ReplaceAllStringFunc(text, func(token string) string {
		id, action, _ := strings.Cut(token, ":")
		if tr.sessions[id] == "" {
			tr.sessions[id] = strconv.Itoa(len(tr.sessions) + 1)
		}
		return tr.sessions[id] + ":" + action
	})
}

// settle sorts the last line into the run of reads of the same route
// before it: the bot makes such reads at once (a chart page's lookups), so
// they arrive in any order.
func (tr *transcript) settle() {
	for i := len(tr.lines) - 1; i > 0; i-- {
		prev, last := tr.lines[i-1], tr.lines[i]
		if readRoute(prev) == "" || readRoute(prev) != readRoute(last) || readOrder(prev) <= readOrder(last) {
			return
		}
		tr.lines[i-1], tr.lines[i] = last, prev
	}
}

const (
	// mediaRoute is where MoviePilot media searches and details are read.
	mediaRoute = "/api/v1/media/"
	// lookups is the route of the reads the bot makes at once (a calendar
	// page, a show's card and seasons): MoviePilot media searches, details
	// and seasons, and Bangumi subjects.
	lookups = "lookups"
)

// tmdbSeason is a TMDB season's episode list, /api/v1/tmdb/{tmdbid}/{season}.
var tmdbSeason = regexp.MustCompile(`^/api/v1/tmdb/\d+/\d+$`)

// readRoute is a read's service and path without its query and trailing
// id, or "" for any other line.
func readRoute(line string) string {
	read, found := strings.CutPrefix(line, "-> ")
	service, path, _ := strings.Cut(read, " GET ")
	if !found || path == "" {
		return ""
	}
	path, query, _ := strings.Cut(path, "?")
	if strings.Contains(query, "type=collection") {
		// a movie's series is searched by all its names at once
		return service + " collections"
	}
	route := strings.TrimRight(path, "0123456789")
	if tmdbSeason.MatchString(path) {
		// the calendar reads every subscribed season at once
		return service + " tmdb seasons"
	}
	if route == mediaRoute || path == mediaRoute+"search" || path == mediaRoute+"seasons" || route == subjectsPath {
		return lookups
	}
	return service + " " + route
}

// readOrder sorts lookups as a pick made one at a time reads them: media
// searches, then Bangumi subjects, then media details.
func readOrder(line string) string {
	for rank, marker := range []string{mediaRoute + "search", subjectsPath, mediaRoute} {
		if strings.Contains(line, marker) {
			return strconv.Itoa(rank) + line
		}
	}
	return line
}

func (tr *transcript) String() string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return strings.Join(tr.lines, "\n") + "\n"
}

// verify compares the transcript with the scenario's golden file, after
// checking that every message the bot showed opens with a heading.
func (tr *transcript) verify(t *testing.T) {
	t.Helper()
	got := tr.String()
	if untitled := untitledMessages(got); len(untitled) > 0 {
		t.Errorf("messages without a heading:\n%s", strings.Join(untitled, "\n"))
	}
	path := filepath.Join("testdata", "transcripts", t.Name()+".txt")
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

// untitledMessages lists the messages sent or edited whose content does not
// open with a heading (posters above it aside), by their first line.
func untitledMessages(transcript string) []string {
	var out []string
	lines := strings.Split(transcript, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "<< sendRichMessage") && !strings.HasPrefix(line, "<< editMessageText") {
			continue
		}
		if first := firstContent(lines[i+1:]); first != "" && !strings.HasPrefix(first, "<h3>") {
			out = append(out, line+"\n"+first)
		}
	}
	return out
}

// firstContent is a message body's first line after its posters.
func firstContent(body []string) string {
	inGallery := false
	for _, line := range body {
		content, ok := strings.CutPrefix(line, "   ")
		if !ok {
			return ""
		}
		switch {
		case content == "<tg-slideshow>":
			inGallery = true
		case content == "</tg-slideshow>":
			inGallery = false
		case inGallery, strings.HasPrefix(content, "<img "):
		default:
			return content
		}
	}
	return ""
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
