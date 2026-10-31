package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"typhonhill.dev/adder/internal/pack"
)

func TestDeprecatedCommandsAndFlags(t *testing.T) {
	saved := [2]map[string]string{deprecatedCommands, deprecatedFlags}
	deprecatedCommands = map[string]string{"adder list": "use adder describe"}
	deprecatedFlags = map[string]string{"adder check --diff": "use adder derive --diff"}
	t.Cleanup(func() { deprecatedCommands, deprecatedFlags = saved[0], saved[1] })

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "adder.yaml"), "version: 1\n")
	tests := []struct {
		name       string
		args       []string
		wantStdout []string // substrings
		wantStderr string
	}{
		{name: "a deprecated command", args: []string{"list"}, wantStderr: "adder: warning: command \"adder list\" is deprecated: use adder describe [deprecated]\n"},
		{name: "a deprecated flag", args: []string{"check", "--diff"}, wantStderr: "adder: warning: flag --diff of \"adder check\" is deprecated: use adder derive --diff [deprecated]\n"},
		{name: "a command without its deprecated flag", args: []string{"check"}},
		{
			name:       "with --json",
			args:       []string{"list", "--json"},
			wantStdout: []string{`"warnings": [` + "\n    {\n" + `      "code": "deprecated",` + "\n" + `      "message": "command \"adder list\" is deprecated: use adder describe"`, `"problems": []`},
		},
		{name: "in the help of the commands", args: []string{"--help"}, wantStdout: []string{"list        List the generators and derived outputs in adder.yaml (deprecated: use adder describe)"}},
		{name: "in the help of the flags", args: []string{"check", "--help"}, wantStdout: []string{"show each out-of-date output as a unified diff (deprecated: use adder derive --diff)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := run(t.Context(), append([]string{"--root", root}, tt.args...)...)
			if code != ExitOK {
				t.Fatalf("exit code = %d, want %d\n%s%s", code, ExitOK, stdout, stderr)
			}
			if stderr != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr, tt.wantStderr)
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout does not contain %q:\n%s", want, stdout)
				}
			}
		})
	}
}

func TestConfigWarnings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a", "adder.yaml"), "version: 1\nderive:\n  scans:\n    Env: {scanner: {name: env}}\n")
	writeFile(t, filepath.Join(root, "b", "adder.yaml"), "version: 1\n")
	t.Chdir(root)
	scanner := func(context.Context, pack.ScanInput) ([]any, error) { return nil, nil }
	packs := []pack.Pack{{Name: "local", Scanners: map[string]pack.Scanner{"env": scanner}, DeprecatedScanners: map[string]string{"env": "use dotenv"}}}
	warning := `scan "Env": scanner "env" is deprecated: use dotenv`

	tests := []struct {
		name       string
		args       []string
		wantStderr string // a substring; empty means no warning is printed
		// wantWarnings is how many warnings the JSON report has at its top level and in each configuration's.
		wantWarnings []int
	}{
		{name: "printed at its place", args: []string{"--root", "a", "check"}, wantStderr: filepath.Join("a", "adder.yaml") + ":4:21: warning: " + warning + " [deprecated]"},
		{name: "in the report", args: []string{"--root", "a", "check", "--json"}, wantWarnings: []int{1}},
		{name: "in the report of its own configuration", args: []string{"check", "./...", "--json"}, wantWarnings: []int{0, 1, 0}},
		{name: "printed once for each configuration", args: []string{"check", "./..."}, wantStderr: filepath.Join("a", "adder.yaml") + ":4:21: warning: " + warning},
		{name: "not printed while completing", args: []string{"--root", "a", "__complete", "new", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Main(t.Context(), "v0.0.0-test", packs, tt.args, strings.NewReader(""), &stdout, &stderr); code != ExitOK {
				t.Fatalf("exit code = %d, want %d\n%s%s", code, ExitOK, stdout.String(), stderr.String())
			}
			switch {
			case tt.wantStderr == "" && strings.Contains(stderr.String(), "warning"):
				t.Errorf("stderr has a warning:\n%s", stderr.String())
			case tt.wantStderr != "" && strings.Count(stderr.String(), tt.wantStderr) != 1:
				t.Errorf("stderr has %q %d times, want once:\n%s", tt.wantStderr, strings.Count(stderr.String(), tt.wantStderr), stderr.String())
			}
			if tt.wantWarnings == nil {
				return
			}
			var rep struct {
				Warnings []struct{ Message string } `json:"warnings"`
				Configs  []struct {
					Warnings []struct{ Message string } `json:"warnings"`
				} `json:"configs"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
				t.Fatalf("%v\n%s", err, stdout.String())
			}
			got := []int{len(rep.Warnings)}
			for _, c := range rep.Configs {
				got = append(got, len(c.Warnings))
			}
			if !slices.Equal(got, tt.wantWarnings) {
				t.Errorf("warnings at the top and in each configuration = %v, want %v\n%s", got, tt.wantWarnings, stdout.String())
			}
		})
	}
}

func TestPackWarningsPrintedOnce(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"a", "b"} {
		writeFile(t, filepath.Join(root, dir, "adder.yaml"), "version: 1\npacks:\n  lists: {pack: store}\n")
	}
	t.Chdir(root)
	store := fstest.MapFS{
		"pack.yaml":    {Data: []byte("version: 1\nderive:\n  outputs:\n    - {template: list.go.tmpl, path: list.gen.go}\n")},
		"list.go.tmpl": {Data: []byte("package x\n\n// {{ shout \"x\" }}\n")},
	}
	packs := []pack.Pack{
		{Name: "store", FS: store},
		{Name: "local", Funcs: map[string]any{"shout": strings.ToUpper}, DeprecatedFuncs: map[string]string{"shout": "use upper"}},
	}
	tests := []struct {
		name string
		args []string
		want int // how many times the warning is printed
	}{
		{name: "one configuration", args: []string{"derive", "./a", "--dry-run"}, want: 1},
		{name: "two configurations using the pack", args: []string{"derive", "./...", "--dry-run"}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Main(t.Context(), "v0.0.0-test", packs, tt.args, strings.NewReader(""), &stdout, &stderr); code != ExitOK {
				t.Fatalf("exit code = %d\n%s%s", code, stdout.String(), stderr.String())
			}
			if got := strings.Count(stderr.String(), `function "shout" is deprecated`); got != tt.want {
				t.Errorf("printed %d times, want %d:\n%s", got, tt.want, stderr.String())
			}
		})
	}
}
