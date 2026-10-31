package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

func TestPrompt(t *testing.T) {
	gcs := filepath.Join("drivers", "gcs", "gcs.go")
	const askName = "name: Driver name, used as the package name\n  must match [a-z][a-z0-9]*; not one of: local, s3\n> "
	tests := []struct {
		name     string
		config   string // replaces adder.yaml when set
		args     []string
		answers  string
		terminal bool
		canceled bool
		wantCode int
		// wantStderr is the exact prompt transcript, when set.
		wantStderr     string
		stderrContains []string
		wantFile       string // a file that must exist afterwards, relative to the root
	}{
		{
			name:       "asks for a missing required input",
			args:       []string{"new", "driver"},
			answers:    "gcs\n",
			terminal:   true,
			wantCode:   ExitOK,
			wantStderr: askName,
			wantFile:   gcs,
		},
		{
			name:     "asks again until the answer is valid",
			args:     []string{"new", "driver"},
			answers:  "\nBad\ns3\n  gcs  \n",
			terminal: true,
			wantCode: ExitOK,
			wantStderr: askName + "  a value is required\n> " + `  input name "Bad" does not match [a-z][a-z0-9]*` + "\n> " +
				`  input name "s3" already exists in Drivers.Package` + "\n> ",
			wantFile: gcs,
		},
		{
			name:     "accepts a number or a value from a list",
			config:   "version: 1\nderive:\n  scans:\n    Drivers: {dirs: [\"drivers/*\"], funcs: {name: ^New$}}\ngenerators:\n  copy:\n    inputs:\n      from: {valuesFrom: Drivers.Package, help: Driver to copy}\n      tier: {values: [fast, slow]}\n    files:\n      - {template: templates/new/README.md.tmpl, path: \"{{ .Inputs.from }}-{{ .Inputs.tier }}.md\"}\n",
			args:     []string{"new", "copy"},
			answers:  "3\n2\nfast\n",
			terminal: true,
			wantCode: ExitOK,
			wantStderr: "from: Driver to copy\n  1) local\n  2) s3\n> " + `  input from "3" is not one of the Drivers.Package values: local, s3` + "\n> " +
				"tier: \n  1) fast\n  2) slow\n> ",
			wantFile: "s3-fast.md",
		},
		{
			name:           "a list a scan leaves empty",
			config:         "version: 1\nderive:\n  scans:\n    None: {dirs: [\"drivers/*\"], funcs: {name: ^Nothing$}}\ngenerators:\n  copy:\n    inputs:\n      from: {valuesFrom: None.Package}\n    files:\n      - {template: templates/new/README.md.tmpl, path: x.md}\n",
			args:           []string{"new", "copy"},
			answers:        "x\n",
			terminal:       true,
			wantCode:       ExitUsage,
			stderrContains: []string{"from: \n  no value is allowed: None.Package has none\n> ", "input from is required [missing-input]"},
		},
		{
			name:           "ends without an answer",
			args:           []string{"new", "driver"},
			terminal:       true,
			wantCode:       ExitUsage,
			stderrContains: []string{askName + "\n", "input name is required [missing-input]"},
		},
		{
			name:           "interrupted",
			args:           []string{"new", "driver"},
			terminal:       true,
			canceled:       true,
			wantCode:       ExitError,
			stderrContains: []string{"adder: interrupted"},
		},
		{
			name:     "does not ask for inputs given as arguments",
			args:     []string{"new", "driver", "name=gcs"},
			terminal: true,
			wantCode: ExitOK,
			wantFile: gcs,
		},
		{
			name:           "does not ask without a terminal",
			args:           []string{"new", "driver"},
			answers:        "gcs\n",
			wantCode:       ExitUsage,
			stderrContains: []string{"input name is required [missing-input]"},
		},
		{
			name:     "does not ask with --json",
			args:     []string{"new", "driver", "--json"},
			answers:  "gcs\n",
			terminal: true,
			wantCode: ExitUsage,
		},
		{
			name:     "does not ask when inputs come from standard input",
			args:     []string{"new", "driver", "--inputs", "-"},
			answers:  `{"summary": "s"}`,
			terminal: true,
			wantCode: ExitUsage,
		},
		{
			name:           "describing the generator fails",
			config:         "version: 1\nderive:\n  scans:\n    Drivers: {dirs: [\"missing/*\"]}\ngenerators:\n  copy:\n    inputs:\n      from: {unique: Drivers.Package}\n    files:\n      - {template: t.tmpl, path: x}\n",
			args:           []string{"new", "copy"},
			terminal:       true,
			wantCode:       ExitError,
			stderrContains: []string{"[no-match]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyDir(t, filepath.Join("..", "scaffold", "testdata", "drivers", "repo"))
			if tt.config != "" {
				writeFile(t, filepath.Join(root, "adder.yaml"), tt.config)
			}
			t.Setenv("NO_COLOR", "1") // the fake terminal would color problem codes
			terminal := isTerminal
			isTerminal = func(any) bool { return tt.terminal }
			t.Cleanup(func() { isTerminal = terminal })
			ctx := t.Context()
			if tt.canceled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			code, _, stderr := runWithInput(ctx, tt.answers, append([]string{"--root", root}, tt.args...)...)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d\nstderr: %s", code, tt.wantCode, stderr)
			}
			if tt.wantStderr != "" && stderr != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr, tt.wantStderr)
			}
			for _, want := range tt.stderrContains {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr = %q, want it to contain %q", stderr, want)
				}
			}
			if tt.wantFile != "" && !exists(t, filepath.Join(root, tt.wantFile)) {
				t.Errorf("%s was not created", tt.wantFile)
			}
		})
	}
}

func TestIsTerminal(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "f"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	tests := []struct {
		name string
		v    any
	}{
		{name: "a reader", v: strings.NewReader("")},
		{name: "a regular file", v: file},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if isTerminal(tt.v) {
				t.Errorf("isTerminal(%s) = true", tt.name)
			}
		})
	}
}

// TestLinesCanceled checks that a prompt waiting for an answer returns when the context is canceled, as with
// Ctrl-C, even though reading never finishes.
func TestLinesCanceled(t *testing.T) {
	blocked, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { writer.Close(); blocked.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	answers := readLines(blocked)
	done := make(chan error)
	go func() {
		_, err := answers.next(ctx)
		done <- err
	}()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("next() error = %v, want context.Canceled", err)
	}
}

func TestLinesReadError(t *testing.T) {
	answers := readLines(iotest.ErrReader(errors.New("broken")))
	if _, err := answers.next(t.Context()); err == nil || err.Error() != "broken" {
		t.Errorf("next() error = %v, want the read error", err)
	}
}
