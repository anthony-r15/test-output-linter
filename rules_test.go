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
		name  string
		lines []string
		// want is what Check returns for the last line fed in; every earlier
		// line must return nil.
		want *Finding
	}{
		{
			name:  "panic summary alone stays pending",
			lines: []string{"panic: boom"},
			want:  nil,
		},
		{
			name:  "panic call in a stack frame, not a summary",
			lines: []string{"panic(0x5a8b40, 0xc0000160c0)", "exit status 2"},
			want:  nil,
		},
		{
			name:  "goroutine dump with no panic before it",
			lines: []string{"goroutine 7 [running]:", "exit status 2"},
			want:  nil,
		},
		{
			name:  "trace ended by exit status",
			lines: []string{"panic: boom", "", "goroutine 7 [running]:", "main.f()", "\t/app/f.go:3 +0x1", "exit status 2"},
			want:  &Finding{Line: 1, Rule: "panic", Severity: SeverityError, Message: "panic: boom (3 stack lines, at /app/f.go:3)"},
		},
		{
			name:  "indented panic header starts a trace",
			lines: []string{"\tpanic: boom", "FAIL"},
			want:  &Finding{Line: 1, Rule: "panic", Severity: SeverityError, Message: "panic: boom"},
		},
		{
			name:  "frame lines with .go paths inside the standard library are not blamed",
			lines: []string{"panic: boom", "\t/usr/local/go/src/runtime/panic.go:838 +0x207", "ok  \texample\t0.1s"},
			want:  &Finding{Line: 1, Rule: "panic", Severity: SeverityError, Message: "panic: boom (1 stack lines)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &PanicRule{}
			var got *Finding
			for i, l := range tt.lines {
				got = r.Check(l, i+1)
				if i < len(tt.lines)-1 && got != nil {
					t.Fatalf("line %d returned %+v before the trace ended", i+1, got)
				}
			}
			assertFindingEqual(t, got, tt.want)
		})
	}
}

func TestPanicRuleFinish(t *testing.T) {
	r := &PanicRule{}
	if f := r.Finish(); f != nil {
		t.Fatalf("Finish with no panic = %+v, want nil", f)
	}
	r.Check("panic: boom", 4)
	assertFindingEqual(t, r.Finish(), &Finding{Line: 4, Rule: "panic", Severity: SeverityError, Message: "panic: boom"})
	if f := r.Finish(); f != nil {
		t.Errorf("second Finish = %+v, want nil once the panic was reported", f)
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
