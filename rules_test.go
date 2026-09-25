package main

import (
	"testing"
	"time"
)

func TestFailRule(t *testing.T) {
	tests := []struct {
		name string
		line string
		want *Finding
	}{
		{
			name: "fail line",
			line: "--- FAIL: TestFoo (0.01s)",
			want: &Finding{Line: 5, Rule: "test-failed", Severity: SeverityError, Message: "test failed: TestFoo (0.01s)"},
		},
		{
			name: "pass line",
			line: "--- PASS: TestFoo (0.01s)",
			want: nil,
		},
		{
			name: "fail line with surrounding whitespace",
			line: "  --- FAIL: TestFoo (0.01s)  ",
			want: &Finding{Line: 5, Rule: "test-failed", Severity: SeverityError, Message: "test failed: TestFoo (0.01s)"},
		},
		{
			name: "bare FAIL summary line, not a per-test result",
			line: "FAIL",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FailRule{}.Check(tt.line, 5)
			assertFindingEqual(t, got, tt.want)
		})
	}
}

func TestPanicRule(t *testing.T) {
	tests := []struct {
		name string
		line string
		want *Finding
	}{
		{
			name: "panic summary line",
			line: "panic: runtime error: index out of range [3] with length 3",
			want: &Finding{Line: 2, Rule: "panic", Severity: SeverityError, Message: "panic: runtime error: index out of range [3] with length 3"},
		},
		{
			name: "panic call in a stack frame, not the summary",
			line: "panic(0x5a8b40, 0xc0000160c0)",
			want: nil,
		},
		{
			name: "goroutine dump line",
			line: "goroutine 7 [running]:",
			want: nil,
		},
		{
			name: "panic line with leading tab",
			line: "\tpanic: boom",
			want: &Finding{Line: 2, Rule: "panic", Severity: SeverityError, Message: "panic: boom"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PanicRule{}.Check(tt.line, 2)
			assertFindingEqual(t, got, tt.want)
		})
	}
}

func TestDataRaceRule(t *testing.T) {
	tests := []struct {
		name string
		line string
		want *Finding
	}{
		{
			name: "warning line",
			line: "WARNING: DATA RACE",
			want: &Finding{Line: 3, Rule: "data-race", Severity: SeverityError, Message: "data race detected"},
		},
		{
			name: "warning embedded in a longer line",
			line: "==== WARNING: DATA RACE ====",
			want: &Finding{Line: 3, Rule: "data-race", Severity: SeverityError, Message: "data race detected"},
		},
		{
			name: "unrelated separator line",
			line: "==================",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DataRaceRule{}.Check(tt.line, 3)
			assertFindingEqual(t, got, tt.want)
		})
	}
}

func TestSkipRuleCountsAcrossCalls(t *testing.T) {
	rule := &SkipRule{}

	got := rule.Check("--- SKIP: TestA (0.00s)", 1)
	assertFindingEqual(t, got, &Finding{Line: 1, Rule: "skip-count", Severity: SeverityInfo, Message: "test skipped: TestA (skip #1)"})

	got = rule.Check("--- PASS: TestB (0.00s)", 2)
	assertFindingEqual(t, got, nil)

	got = rule.Check("--- SKIP: TestC (0.00s)", 3)
	assertFindingEqual(t, got, &Finding{Line: 3, Rule: "skip-count", Severity: SeverityInfo, Message: "test skipped: TestC (skip #2)"})
}

func TestDuplicateTestNameRule(t *testing.T) {
	rule := &DuplicateTestNameRule{}

	got := rule.Check("--- FAIL: TestFlaky (0.00s)", 1)
	assertFindingEqual(t, got, nil)

	got = rule.Check("--- PASS: TestStable (0.00s)", 2)
	assertFindingEqual(t, got, nil)

	got = rule.Check("--- PASS: TestFlaky (0.01s)", 3)
	assertFindingEqual(t, got, &Finding{
		Line:     3,
		Rule:     "flaky-rerun",
		Severity: SeverityWarning,
		Message:  "TestFlaky reported a result more than once (PASS again here), possible flaky rerun",
	})

	// TestStable only reported once so far and must stay quiet.
	got = rule.Check("--- SKIP: TestUnrelated (0.00s)", 4)
	assertFindingEqual(t, got, nil)
}

func TestSlowTestRule(t *testing.T) {
	rule := SlowTestRule{Threshold: time.Second}

	tests := []struct {
		name string
		line string
		want *Finding
	}{
		{
			name: "over threshold",
			line: "--- PASS: TestFullSync (1.50s)",
			want: &Finding{Line: 7, Rule: "slow-test", Severity: SeverityWarning, Message: "TestFullSync took 1.5s, over threshold 1s"},
		},
		{
			name: "under threshold",
			line: "--- PASS: TestQuick (0.50s)",
			want: nil,
		},
		{
			name: "exactly at threshold still flags",
			line: "--- FAIL: TestBorderline (1s)",
			want: &Finding{Line: 7, Rule: "slow-test", Severity: SeverityWarning, Message: "TestBorderline took 1s, over threshold 1s"},
		},
		{
			name: "malformed duration",
			line: "--- PASS: TestWeird (abc)",
			want: nil,
		},
		{
			name: "no duration at all",
			line: "--- PASS: TestNoDuration",
			want: nil,
		},
		{
			name: "skip line is not a duration-bearing result",
			line: "--- SKIP: TestSkipped (0.00s)",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rule.Check(tt.line, 7)
			assertFindingEqual(t, got, tt.want)
		})
	}
}

func assertFindingEqual(t *testing.T, got, want *Finding) {
	t.Helper()
	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Fatalf("got %+v, want %+v", got, want)
	case *got != *want:
		t.Fatalf("got %+v, want %+v", *got, *want)
	}
}
