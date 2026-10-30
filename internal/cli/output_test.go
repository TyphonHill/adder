package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/plan"
)

func TestUseColor(t *testing.T) {
	tests := []struct {
		name     string
		terminal bool
		noColor  string
		term     string
		console  bool // whether the console's color support turns on
		want     bool
	}{
		{name: "a terminal", terminal: true, term: "xterm-256color", console: true, want: true},
		{name: "not a terminal", terminal: false, term: "xterm-256color", console: true, want: false},
		{name: "NO_COLOR", terminal: true, noColor: "1", term: "xterm-256color", console: true, want: false},
		{name: "an empty NO_COLOR does not count", terminal: true, noColor: "", term: "xterm", console: true, want: true},
		{name: "a dumb terminal", terminal: true, term: "dumb", console: true, want: false},
		{name: "a console without color support", terminal: true, term: "xterm", console: false, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tt.noColor)
			t.Setenv("TERM", tt.term)
			terminal, console := isTerminal, enableColor
			isTerminal = func(any) bool { return tt.terminal }
			enableColor = func(io.Writer) bool { return tt.console }
			t.Cleanup(func() { isTerminal, enableColor = terminal, console })
			if got := useColor(&bytes.Buffer{}); got != tt.want {
				t.Errorf("useColor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrinter(t *testing.T) {
	created := plan.Change{Path: "a.go", Status: plan.Created}
	unchanged := plan.Change{Path: "b.go", Status: plan.Unchanged}
	skipped := plan.Change{Path: "c.md", Status: plan.Skipped}
	tests := []struct {
		name    string
		printer printer
		changes []plan.Change
		planned bool
		want    string
	}{
		{name: "changes", changes: []plan.Change{created, unchanged}, want: "created    a.go\n"},
		{name: "planned changes", changes: []plan.Change{created, skipped}, planned: true, want: "would be created    a.go\nskipped    c.md\n"},
		{name: "verbose", printer: printer{verbose: true}, changes: []plan.Change{unchanged}, want: "unchanged  b.go\n"},
		{name: "quiet", printer: printer{quiet: true}, changes: []plan.Change{created}, want: ""},
		{name: "color", printer: printer{color: true}, changes: []plan.Change{created}, want: "\x1b[32mcreated  \x1b[0m  a.go\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := tt.printer
			p.w = &buf
			for _, c := range tt.changes {
				p.change(c, tt.planned)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintErrorColor(t *testing.T) {
	err := &diag.Diagnostic{Code: diag.GoSyntax, Pos: diag.Pos{File: "a.go", Line: 1}, Message: "bad", Hint: "fix it"}
	var plain, colored bytes.Buffer
	printError(&plain, err, false)
	printError(&colored, err, true)
	if want := "a.go:1: bad [go-syntax]\n\thint: fix it\n"; plain.String() != want {
		t.Errorf("plain = %q, want %q", plain.String(), want)
	}
	if want := "a.go:1: bad \x1b[31m[go-syntax]\x1b[0m\n\t\x1b[2mhint:\x1b[0m fix it\n"; colored.String() != want {
		t.Errorf("colored = %q, want %q", colored.String(), want)
	}
	var usage bytes.Buffer
	printError(&usage, &usageError{errors.New("unknown flag: --x"), "adder derive"}, true)
	if strings.Contains(usage.String(), "\x1b[") {
		t.Errorf("usage errors are not colored: %q", usage.String())
	}
}
