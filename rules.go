package main

import (
	"fmt"
	"strings"
	"time"
)

// FailRule flags every "--- FAIL:" line go test prints for a failed test.
type FailRule struct{}

func (FailRule) Name() string { return "test-failed" }

func (FailRule) Check(line string, lineNo int) *Finding {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "--- FAIL:") {
		return nil
	}
	name := strings.TrimSpace(strings.TrimPrefix(trimmed, "--- FAIL:"))
	return &Finding{
		Line:     lineNo,
		Rule:     "test-failed",
		Severity: SeverityError,
		Message:  fmt.Sprintf("test failed: %s", name),
	}
}

// PanicRule flags panics surfaced in test output. go test does not mark
// these with a "--- FAIL:" line of their own, so without this rule a
// panicking test can scroll past unnoticed in a long log.
type PanicRule struct{}

func (PanicRule) Name() string { return "panic" }

func (PanicRule) Check(line string, lineNo int) *Finding {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "panic:") {
		return nil
	}
	return &Finding{
		Line:     lineNo,
		Rule:     "panic",
		Severity: SeverityError,
		Message:  trimmed,
	}
}

// DataRaceRule flags races reported by the race detector (go test -race).
type DataRaceRule struct{}

func (DataRaceRule) Name() string { return "data-race" }

func (DataRaceRule) Check(line string, lineNo int) *Finding {
	if !strings.Contains(line, "WARNING: DATA RACE") {
		return nil
	}
	return &Finding{
		Line:     lineNo,
		Rule:     "data-race",
		Severity: SeverityError,
		Message:  "data race detected",
	}
}

// testResultPrefixes maps the "--- STATUS:" prefix go test prints for a
// finished test to the status name, in the order Check should try them.
var testResultPrefixes = [...]string{"PASS", "FAIL", "SKIP"}

// parseTestResult extracts the test name and status from a "--- PASS:",
// "--- FAIL:", or "--- SKIP:" line. trimmed must already have leading and
// trailing whitespace removed.
func parseTestResult(trimmed string) (name, status string, ok bool) {
	for _, status := range testResultPrefixes {
		prefix := "--- " + status + ":"
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		if idx := strings.Index(rest, "("); idx != -1 {
			rest = strings.TrimSpace(rest[:idx])
		}
		return rest, status, true
	}
	return "", "", false
}

// SkipRule flags every "--- SKIP:" line and tracks how many have been seen
// so far, so a run with a growing number of skipped tests stands out even
// though no single skip is an error on its own.
type SkipRule struct {
	count int
}

func (*SkipRule) Name() string { return "skip-count" }

func (r *SkipRule) Check(line string, lineNo int) *Finding {
	name, status, ok := parseTestResult(strings.TrimSpace(line))
	if !ok || status != "SKIP" {
		return nil
	}
	r.count++
	return &Finding{
		Line:     lineNo,
		Rule:     "skip-count",
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("test skipped: %s (skip #%d)", name, r.count),
	}
}

// DuplicateTestNameRule flags a test name that reports a PASS, FAIL, or
// SKIP result more than once in the same log. A well-behaved single `go
// test` invocation reports each test exactly once, so a repeat usually
// means the test (or the whole run) was retried after failing - a sign of
// a flaky test rather than a stable one, even if the retry passed.
type DuplicateTestNameRule struct {
	seen map[string]int
}

func (*DuplicateTestNameRule) Name() string { return "flaky-rerun" }

func (r *DuplicateTestNameRule) Check(line string, lineNo int) *Finding {
	name, status, ok := parseTestResult(strings.TrimSpace(line))
	if !ok || name == "" {
		return nil
	}
	if r.seen == nil {
		r.seen = make(map[string]int)
	}
	r.seen[name]++
	if r.seen[name] < 2 {
		return nil
	}
	return &Finding{
		Line:     lineNo,
		Rule:     "flaky-rerun",
		Severity: SeverityWarning,
		Message:  fmt.Sprintf("%s reported a result more than once (%s again here), possible flaky rerun", name, status),
	}
}

// SlowTestRule flags tests whose reported duration is at or above
// Threshold. Both "--- PASS:" and "--- FAIL:" lines carry a duration, so a
// slow test is flagged regardless of outcome.
type SlowTestRule struct {
	Threshold time.Duration
}

func (SlowTestRule) Name() string { return "slow-test" }

func (r SlowTestRule) Check(line string, lineNo int) *Finding {
	trimmed := strings.TrimSpace(line)
	var rest string
	switch {
	case strings.HasPrefix(trimmed, "--- PASS:"):
		rest = strings.TrimPrefix(trimmed, "--- PASS:")
	case strings.HasPrefix(trimmed, "--- FAIL:"):
		rest = strings.TrimPrefix(trimmed, "--- FAIL:")
	default:
		return nil
	}

	openIdx := strings.LastIndex(rest, "(")
	closeIdx := strings.LastIndex(rest, ")")
	if openIdx == -1 || closeIdx == -1 || closeIdx < openIdx {
		return nil
	}
	dur, err := time.ParseDuration(rest[openIdx+1 : closeIdx])
	if err != nil || dur < r.Threshold {
		return nil
	}
	return &Finding{
		Line:     lineNo,
		Rule:     "slow-test",
		Severity: SeverityWarning,
		Message:  fmt.Sprintf("%s took %s, over threshold %s", strings.TrimSpace(rest[:openIdx]), dur, r.Threshold),
	}
}
