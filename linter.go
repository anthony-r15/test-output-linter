package main

import (
	"bufio"
	"io"
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

// Lint reads r line by line and runs every rule against each line, calling
// emit for every match. It never holds more than the current line in
// memory, so it can run against arbitrarily large test output piped from a
// long CI job without buffering the whole thing.
func Lint(r io.Reader, rules []Rule, emit Emitter) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		for _, rule := range rules {
			if f := rule.Check(line, lineNo); f != nil {
				emit(*f)
			}
		}
	}
	return scanner.Err()
}
