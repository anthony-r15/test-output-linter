package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// maxLineSize bounds how much a single line can grow the scanner's buffer.
// go test can emit long lines (assertion diffs, JSON blobs) but a run with
// gigabytes of test output should still lint in constant memory, so we cap
// a single line rather than letting it grow unbounded.
const maxLineSize = 1 << 20 // 1 MiB

// Severity ranks how serious a Finding is.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	default:
		return "unknown"
	}
}

// Finding is a single lint result tied to the line of input it came from.
type Finding struct {
	Line     int
	Rule     string
	Severity Severity
	Message  string
}

// Rule inspects one line of test output at a time and optionally reports a
// Finding for it. Rules are stateless across lines: everything a rule needs
// (test name, duration, ...) is expected to live on the line itself, which
// is what keeps the linter able to stream.
type Rule interface {
	Name() string
	Check(line string, lineNo int) *Finding
}

// Emitter receives findings as they are discovered.
type Emitter func(Finding)

// testEvent mirrors the subset of the record shape emitted by
// `go test -json` that rules care about: one JSON object per line, each
// wrapping either a single line of the same text -v would have printed
// (Action == "output") or a bare status change with no text to check
// (Action == "pass", "fail", "run", ...).
type testEvent struct {
	Action string
	Output string
}

// decodeTestEvent reports whether line is a `go test -json` event. Plain -v
// output never starts with '{', so this is enough to tell the two input
// formats apart line by line without any upfront mode flag.
func decodeTestEvent(line string) (testEvent, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || trimmed[0] != '{' {
		return testEvent{}, false
	}
	var ev testEvent
	if err := json.Unmarshal([]byte(trimmed), &ev); err != nil || ev.Action == "" {
		return testEvent{}, false
	}
	return ev, true
}

// Lint reads r line by line and runs every rule against each line, calling
// emit for every match. It never holds more than the current line in
// memory, so it can run against arbitrarily large test output piped from a
// long CI job without buffering the whole thing.
//
// Input may be plain `go test -v` text or `go test -json` output; the two
// can even be mixed line by line, since each line is classified on its own.
// For JSON input, rules see the text carried in each "output" event's
// Output field rather than the raw JSON, so the same rules work against
// both formats unchanged. Other JSON event types (pass, fail, run, ...)
// carry no such text and are skipped.
func Lint(r io.Reader, rules []Rule, emit Emitter) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if ev, ok := decodeTestEvent(line); ok {
			if ev.Action != "output" {
				continue
			}
			line = strings.TrimRight(ev.Output, "\n")
		}
		for _, rule := range rules {
			if f := rule.Check(line, lineNo); f != nil {
				emit(*f)
			}
		}
	}
	return scanner.Err()
}
