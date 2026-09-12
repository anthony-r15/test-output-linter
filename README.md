# testlint

A linter for `go test` output, not for Go source. It reads the text a test
run prints and reports findings with the line number they occurred on, so a
failure buried three thousand lines into a CI log can be jumped to directly
instead of scrolled to by eye.

The output of `go test -v ./...` mixes a lot of things together: passes,
failures, panics, race warnings, and whatever the tests themselves log. On a
large repo that log can run to tens of thousands of lines, and the signal
(the one test that actually broke) is easy to miss in the noise. testlint
scans that stream and flags the lines worth looking at.

## Usage

Pipe test output straight in:

```sh
go test -v ./... | testlint
```

Or lint a saved log:

```sh
go test -v ./... > run.log
testlint run.log
```

`go test -json` output works the same way:

```sh
go test -json ./... | testlint
```

testlint tells the two formats apart per line, so it never needs to be told
which one it's reading.

Example output:

```
142: [error] test-failed: test failed: TestParseConfig
143: [error] panic: panic: runtime error: index out of range [3] with length 3
980: [warning] slow-test: TestFullSync took 4.2s, over threshold 1s
```

Exit status is `1` if any error-level finding was reported, `0` otherwise,
so it can gate a CI step:

```sh
go test -v ./... | testlint || exit 1
```

## Flags

```sh
testlint -slow-threshold=2s -rules=test-failed,panic run.log
```

- `-slow-threshold` sets how long a test must run to be flagged by
  `slow-test`. Default `1s`.
- `-rules` picks which rules run, as a comma-separated list of rule names.
  Default is all of them: `test-failed,panic,data-race,slow-test,skip-count,flaky-rerun`.

## Why streaming matters here

Test logs from a real CI job can be large enough that reading the whole
thing into memory before looking at it is wasteful, or on a big enough
monorepo, a real problem. testlint never does that: it reads one line at a
time with a bounded scanner buffer, checks that line against each rule, and
moves on. Memory use stays flat whether the input is a hundred lines or a
hundred million.

## Rules

| rule | severity | triggers on |
|---|---|---|
| `test-failed` | error | a `--- FAIL:` line |
| `panic` | error | a `panic:` line |
| `data-race` | error | `WARNING: DATA RACE` (from `go test -race`) |
| `slow-test` | warning | a `--- PASS:`/`--- FAIL:` line whose duration is >= 1s |
| `skip-count` | info | a `--- SKIP:` line, numbered by how many have been seen |
| `flaky-rerun` | warning | the same test name reporting PASS/FAIL/SKIP more than once |

## Building

```sh
go build -o testlint .
```

Standard library only, no external dependencies.
