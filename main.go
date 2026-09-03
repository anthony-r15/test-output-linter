// Command testlint reads go test output and reports findings with the
// line number they occurred on, so a failure buried in a long CI log can
// be jumped to directly instead of scrolled to.
package main

import (
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	var in io.Reader = os.Stdin
	if len(os.Args) > 1 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "testlint:", err)
			os.Exit(2)
		}
		defer f.Close()
		in = f
	}

	rules := []Rule{
		FailRule{},
		PanicRule{},
		DataRaceRule{},
		SlowTestRule{Threshold: time.Second},
	}

	sawError := false
	emit := func(f Finding) {
		if f.Severity == SeverityError {
			sawError = true
		}
		fmt.Printf("%d: [%s] %s: %s\n", f.Line, f.Severity, f.Rule, f.Message)
	}

	if err := Lint(in, rules, emit); err != nil {
		fmt.Fprintln(os.Stderr, "testlint:", err)
		os.Exit(2)
	}

	if sawError {
		os.Exit(1)
	}
}
