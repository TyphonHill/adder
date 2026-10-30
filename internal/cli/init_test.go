package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/TyphonHill/adder/internal/config"
)

// commentedYAML matches a commented-out line of YAML in the starter configuration, but not prose.
var commentedYAML = regexp.MustCompile(`(?m)^(\s*)# (\s*(?:[A-Za-z]+:|- ).*)$`)

func TestStarterConfig(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		complete bool // every section has content
	}{
		{name: "as written", data: starterConfig},
		{name: "with every example uncommented", data: commentedYAML.ReplaceAllString(starterConfig, "$1$2"), complete: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Parse(config.FileName, []byte(tt.data))
			if err != nil {
				t.Fatalf("Parse() error = %v\n%s", err, tt.data)
			}
			if tt.complete && (cfg.Vars == nil || len(cfg.Derive.Scans) == 0 || len(cfg.Derive.Outputs) == 0 || len(cfg.Generators) == 0) {
				t.Errorf("uncommenting left a section empty:\n%s", tt.data)
			}
		})
	}
}

func TestInit(t *testing.T) {
	tests := []struct {
		name string
		// setup prepares the empty directory at root; it may be nil.
		setup          func(t *testing.T, root string)
		args           func(root string) []string
		dir            bool // run in root without flags
		wantCode       int
		wantStdout     string
		stderrContains []string
		wantFile       string // relative to root, holding the starter configuration afterwards
		wantUntouched  string // the content adder.yaml must still have afterwards
	}{
		{name: "in the current directory", dir: true, args: func(string) []string { return []string{"init"} }, wantCode: ExitOK, wantStdout: "created    adder.yaml\n", wantFile: "adder.yaml"},
		{name: "in the root", args: func(root string) []string { return []string{"--root", root, "init"} }, wantCode: ExitOK, wantFile: "adder.yaml"},
		{
			name:     "at the configuration path",
			args:     func(root string) []string { return []string{"--config", filepath.Join(root, "custom.yaml"), "init"} },
			wantCode: ExitOK,
			wantFile: "custom.yaml",
		},
		{
			name: "never overwrites",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "adder.yaml"), "mine\n")
			},
			args:           func(root string) []string { return []string{"--root", root, "init"} },
			wantCode:       ExitError,
			stderrContains: []string{"adder.yaml: file already exists [file-exists]"},
			wantUntouched:  "mine\n",
		},
		{
			name:           "missing directory",
			args:           func(root string) []string { return []string{"--root", filepath.Join(root, "missing"), "init"} },
			wantCode:       ExitError,
			stderrContains: []string{"[io]"},
		},
		{name: "arguments", args: func(string) []string { return []string{"init", "x"} }, wantCode: ExitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, root)
			}
			if tt.dir {
				t.Chdir(root)
			}

			code, stdout, stderr := run(t.Context(), tt.args(root)...)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d\nstderr: %s", code, tt.wantCode, stderr)
			}
			if tt.wantStdout != "" && stdout != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tt.wantStdout)
			}
			for _, want := range tt.stderrContains {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr = %q, want it to contain %q", stderr, want)
				}
			}
			if tt.wantFile != "" {
				if got, err := os.ReadFile(filepath.Join(root, tt.wantFile)); err != nil || string(got) != starterConfig {
					t.Errorf("%s = %q, %v, want the starter configuration", tt.wantFile, got, err)
				}
			}
			if tt.wantUntouched != "" {
				if got, err := os.ReadFile(filepath.Join(root, "adder.yaml")); string(got) != tt.wantUntouched {
					t.Errorf("adder.yaml = %q, %v, want it untouched", got, err)
				}
			}
		})
	}
}
