package moviepilot

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"unicode/utf8"
)

const maxSSEFrameBytes = 8 * 1024 * 1024

// SSE is a sequence of blank-line-delimited UTF-8 frames, not reads or JSON
// lines. Comments and id/retry fields never cause another upstream request.
type sseReader struct {
	scanner *bufio.Scanner
	first   bool
}

func newSSEReader(body io.Reader) *sseReader {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(nil, maxSSEFrameBytes)
	scanner.Split(sseLine)
	return &sseReader{scanner: scanner, first: true}
}

func (r *sseReader) next() ([]byte, error) {
	var data strings.Builder
	for r.scanner.Scan() {
		line := r.scanner.Text()
		if r.first {
			line, r.first = strings.TrimPrefix(line, "\ufeff"), false
		}
		if !utf8.ValidString(line) {
			return nil, errSSEProtocol
		}
		if line == "" && data.Len() > 0 {
			return []byte(strings.TrimSuffix(data.String(), "\n")), nil
		}
		field, value, _ := strings.Cut(line, ":")
		if field != "data" {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		if data.Len()+len(value)+1 > maxSSEFrameBytes {
			return nil, errSSEProtocol
		}
		data.WriteString(value)
		data.WriteByte('\n')
	}
	// An EOF, even after a complete JSON value but without its blank line,
	// never dispatches a pending event or proves successful completion.
	return nil, io.ErrUnexpectedEOF
}

// Accept CRLF, bare CR and bare LF, including CRLF split across network reads.
func sseLine(data []byte, atEOF bool) (int, []byte, error) {
	i := bytes.IndexAny(data, "\r\n")
	if i < 0 {
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	}
	if data[i] == '\n' {
		return i + 1, data[:i], nil
	}
	if i+1 == len(data) && !atEOF {
		return 0, nil, nil
	}
	advance := i + 1
	if advance < len(data) && data[advance] == '\n' {
		advance++
	}
	return advance, data[:i], nil
}
