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
