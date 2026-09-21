package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// Formatter renders a single finding as it arrives. Implementations must
// not buffer findings across calls, so output stays as streamable as the
// linter itself.
type Formatter interface {
	Write(w io.Writer, f Finding) error
}

// TextTableFormatter renders each finding as one aligned row: line number,
// severity, rule name, message. Column widths are fixed ahead of time from
// the known severity and rule name lengths rather than measured from the
// data, so a row can be written the moment its finding arrives instead of
// waiting to see the widest value in the run.
type TextTableFormatter struct{}

func (TextTableFormatter) Write(w io.Writer, f Finding) error {
	_, err := fmt.Fprintf(w, "%6d  %-7s  %-12s  %s\n", f.Line, f.Severity, f.Rule, f.Message)
	return err
}

// JSONFormatter renders each finding as one JSON object per line (JSON
// Lines), so a consumer can parse findings as they arrive instead of
// waiting for a closing array bracket at the end of the run.
type JSONFormatter struct{}

// jsonFinding mirrors Finding but spells Severity out as text; the int
// values of Severity are an implementation detail rules and formatters
// share, not something worth exposing to consumers of the JSON output.
type jsonFinding struct {
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
}

func (JSONFormatter) Write(w io.Writer, f Finding) error {
	return json.NewEncoder(w).Encode(jsonFinding{
		Line:     f.Line,
		Severity: f.Severity.String(),
		Rule:     f.Rule,
		Message:  f.Message,
	})
}

// formatters maps the name a -format flag accepts to the Formatter it
// selects.
var formatters = map[string]Formatter{
	"text": TextTableFormatter{},
	"json": JSONFormatter{},
}
