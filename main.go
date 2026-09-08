// Command testlint reads go test output and reports findings with the
// line number they occurred on, so a failure buried in a long CI log can
// be jumped to directly instead of scrolled to.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// allRules returns every rule testlint knows about, keyed by the name it
// reports findings under, so -rules can select a subset by that same name.
func allRules(slowThreshold time.Duration) map[string]Rule {
	return map[string]Rule{
		"test-failed": FailRule{},
		"panic":       PanicRule{},
		"data-race":   DataRaceRule{},
		"slow-test":   SlowTestRule{Threshold: slowThreshold},
	}
}

func main() {
	slowThreshold := flag.Duration("slow-threshold", time.Second, "minimum test duration to flag as slow")
	ruleNames := flag.String("rules", "test-failed,panic,data-race,slow-test", "comma-separated list of rules to run")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: testlint [flags] [file]")
		fmt.Fprintln(os.Stderr, "reads from stdin if no file is given")
		flag.PrintDefaults()
	}
	flag.Parse()

	var in io.Reader = os.Stdin
	if flag.NArg() > 0 {
		f, err := os.Open(flag.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, "testlint:", err)
			os.Exit(2)
		}
		defer f.Close()
		in = f
	}

	available := allRules(*slowThreshold)
	var rules []Rule
	for _, name := range strings.Split(*ruleNames, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		rule, ok := available[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "testlint: unknown rule %q\n", name)
			os.Exit(2)
		}
		rules = append(rules, rule)
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
