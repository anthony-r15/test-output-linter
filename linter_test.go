package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// lintRules returns one instance of every rule in the fixed order the
// transcript tests below assert findings in. Stateful rules (SkipRule,
// DuplicateTestNameRule) must be built fresh per call so tests don't leak
// counters into each other.
func lintRules() []Rule {
	return []Rule{
		FailRule{},
		PanicRule{},
		DataRaceRule{},
		SlowTestRule{Threshold: time.Second},
		&SkipRule{},
		&DuplicateTestNameRule{},
	}
}

func lintFile(t *testing.T, path string) []Finding {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	var findings []Finding
	if err := Lint(f, lintRules(), func(fnd Finding) { findings = append(findings, fnd) }); err != nil {
		t.Fatalf("lint %s: %v", path, err)
	}
	return findings
}

// TestLintVerboseMixedTranscript runs the full rule set over a recorded
// `go test -v` transcript covering a pass, a fail, a slow pass, a skip, and
// a test that fails then passes on rerun, and checks every finding the run
// should produce, in the order rules see them.
func TestLintVerboseMixedTranscript(t *testing.T) {
	got := lintFile(t, "testdata/verbose_mixed.txt")
	want := []Finding{
		{Line: 4, Rule: "test-failed", Severity: SeverityError, Message: "test failed: TestParseConfig (0.01s)"},
		{Line: 7, Rule: "slow-test", Severity: SeverityWarning, Message: "TestFullSync took 4.2s, over threshold 1s"},
		{Line: 9, Rule: "skip-count", Severity: SeverityInfo, Message: "test skipped: TestShortModeOnly (skip #1)"},
		{Line: 12, Rule: "test-failed", Severity: SeverityError, Message: "test failed: TestFlakyRetry (0.02s)"},
		{Line: 15, Rule: "flaky-rerun", Severity: SeverityWarning, Message: "TestFlakyRetry reported a result more than once (PASS again here), possible flaky rerun"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings mismatch:\n got:  %+v\n want: %+v", got, want)
	}
}

// TestLintPanicTranscript checks that a panic is flagged on the line
// bearing the "panic:" summary, not on the goroutine dump or frame lines
// that follow it.
func TestLintPanicTranscript(t *testing.T) {
	got := lintFile(t, "testdata/panic.txt")
	want := []Finding{
		{Line: 2, Rule: "panic", Severity: SeverityError, Message: "panic: runtime error: index out of range [3] with length 3"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings mismatch:\n got:  %+v\n want: %+v", got, want)
	}
}

// TestLintDataRaceTranscript checks that a `go test -race` report is
// flagged on its "WARNING: DATA RACE" line and the test is still flagged
// as failed on its own "--- FAIL:" line further down.
func TestLintDataRaceTranscript(t *testing.T) {
	got := lintFile(t, "testdata/data_race.txt")
	want := []Finding{
		{Line: 3, Rule: "data-race", Severity: SeverityError, Message: "data race detected"},
		{Line: 12, Rule: "test-failed", Severity: SeverityError, Message: "test failed: TestConcurrentWrite (0.00s)"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings mismatch:\n got:  %+v\n want: %+v", got, want)
	}
}

// TestLintJSONTranscript checks that findings from `go test -json` input
// land on the line number of the JSON record itself (not some offset into
// the text it carries), and that non-"output" records like "run"/"pass"/
// "fail" are skipped rather than checked as text.
func TestLintJSONTranscript(t *testing.T) {
	got := lintFile(t, "testdata/json_events.txt")
	want := []Finding{
		{Line: 7, Rule: "test-failed", Severity: SeverityError, Message: "test failed: TestParseConfig (0.01s)"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings mismatch:\n got:  %+v\n want: %+v", got, want)
	}
}

func TestLintLineTooLong(t *testing.T) {
	huge := strings.NewReader(strings.Repeat("x", maxLineSize+1))
	err := Lint(huge, lintRules(), func(Finding) {})
	if err == nil {
		t.Fatal("expected an error for a line past maxLineSize, got nil")
	}
}

func TestDecodeTestEvent(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		wantOK bool
		want   testEvent
	}{
		{
			name:   "plain text line",
			line:   "--- PASS: TestAdd (0.00s)",
			wantOK: false,
		},
		{
			name:   "json output event",
			line:   `{"Action":"output","Output":"--- PASS: TestAdd (0.00s)\n"}`,
			wantOK: true,
			want:   testEvent{Action: "output", Output: "--- PASS: TestAdd (0.00s)\n"},
		},
		{
			name:   "json status event with no output text",
			line:   `{"Action":"pass","Test":"TestAdd"}`,
			wantOK: true,
			want:   testEvent{Action: "pass"},
		},
		{
			name:   "malformed json",
			line:   `{"Action":"output"`,
			wantOK: false,
		},
		{
			name:   "json object missing Action",
			line:   `{"Output":"no action here\n"}`,
			wantOK: false,
		},
		{
			name:   "blank line",
			line:   "   ",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := decodeTestEvent(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("decodeTestEvent(%q) ok = %v, want %v", tt.line, ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("decodeTestEvent(%q) = %+v, want %+v", tt.line, got, tt.want)
			}
		})
	}
}
