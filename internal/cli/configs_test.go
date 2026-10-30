package cli

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// monorepo writes a repository with a root configuration and two nested ones, and changes into it.
func monorepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	const cfg = "version: 1\nderive:\n  scans:\n    Cmds: {dirs: [\"**\"], funcs: {name: ^Run$}}\n  outputs:\n    - {template: list.tmpl, path: cmds.md}\n"
	files := map[string]string{
		"go.mod":                        "module example.com/mono\n",
		"adder.yaml":                    cfg,
		"list.tmpl":                     "{{ range .Cmds }}- {{ .ImportPath }}\n{{ end }}",
		"tools/t.go":                    "package tools\n\nfunc Run() {}\n",
		"services/api/adder.yaml":       cfg + "generators:\n  api:\n    files: [{template: list.tmpl, path: x.md}]\n    tests: [{name: plain, want: testdata/plain}]\n",
		"services/api/list.tmpl":        "{{ range .Cmds }}- {{ .ImportPath }}\n{{ end }}",
		"services/api/go.mod":           "module example.com/api\n",
		"services/api/h/h.go":           "package h\n\nfunc Run() {}\n",
		"services/web/adder.yaml":       cfg,
		"services/web/list.tmpl":        "{{ range .Cmds }}- {{ .File }}\n{{ end }}",
		"services/web/w/w.go":           "package w\n\nfunc Run() {}\n",
		"testdata/fixture/adder.yaml":   "not: a configuration\n",
		".hidden/adder.yaml":            "not: a configuration\n",
		"docs/README.md":                "nothing to see\n",
		"services/api/testdata/plain/x": "x",
	}
	for name, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(name)), content)
	}
	t.Chdir(root)
	return root
}

func TestConfigs(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T, root string)
		args           []string
		wantCode       int
		wantStdout     string
		stdoutContains []string
		stderrContains []string
	}{
		{
			name:       "every configuration below the directory",
			args:       []string{"derive", "./..."},
			wantStdout: "created    cmds.md\ncreated    services/api/cmds.md\ncreated    services/web/cmds.md\n",
		},
		{
			name:       "one configuration by directory",
			args:       []string{"derive", "services/api"},
			wantStdout: "created    services/api/cmds.md\n",
		},
		{
			name:       "a pattern below the current directory",
			args:       []string{"derive", "./services/..."},
			wantStdout: "created    services/api/cmds.md\ncreated    services/web/cmds.md\n",
		},
		{
			name:           "verbose names each configuration's scans",
			args:           []string{"check", "-v", "./services/..."},
			wantCode:       ExitStale,
			stdoutContains: []string{"scan Cmds in services/api found 1 item\n", "would be created    services/web/cmds.md\n"},
			stderrContains: []string{"2 derived files are out of date"},
		},
		{
			name:           "diffs apply from the current directory",
			args:           []string{"check", "--diff", "services/web"},
			wantCode:       ExitStale,
			stdoutContains: []string{"diff /dev/null b/services/web/cmds.md\n"},
		},
		{
			name: "a failing configuration fails the run, and the others still run",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "services", "web", "adder.yaml"), "version: 1\nderivee: {}\n")
			},
			args:           []string{"derive", "./..."},
			wantCode:       ExitError,
			stdoutContains: []string{"created    services/api/cmds.md\n"},
			stderrContains: []string{"unknown field \"derivee\" [unknown-field]"},
		},
		{
			name:           "list prints each configuration",
			args:           []string{"list", "./..."},
			stdoutContains: []string{"adder.yaml\ngenerators\n  none\n", "services/api/adder.yaml\ngenerators\n  api  \n"},
		},
		{
			name:           "test runs each configuration's tests",
			args:           []string{"test", "./...", "api"},
			wantCode:       ExitError,
			stdoutContains: []string{"FAIL  services/api/api/plain\n"},
			stderrContains: []string{"[golden-mismatch]"},
		},
		{
			name:           "test with a generator no configuration has",
			args:           []string{"test", "./...", "nope"},
			wantCode:       ExitUsage,
			stderrContains: []string{`unknown generator "nope"`},
		},
		{
			name:           "test over directories named one by one",
			args:           []string{"test", "services/api", "services/web"},
			wantCode:       ExitError,
			stderrContains: []string{"[golden-mismatch]"},
		},
		{
			name:           "patterns with --root",
			args:           []string{"check", "--root", ".", "./..."},
			wantCode:       ExitUsage,
			stderrContains: []string{"--root and --config cannot be combined with directory patterns"},
		},
		{
			name:           "an argument that is not a directory",
			args:           []string{"list", "services"},
			wantCode:       ExitUsage,
			stderrContains: []string{`"services" is not a directory`},
		},
		{
			name:           "--out with patterns",
			args:           []string{"derive", "--out", "x", "./..."},
			wantCode:       ExitUsage,
			stderrContains: []string{"--out writes one configuration's outputs"},
		},
		{
			name:           "a pattern that matches nothing",
			args:           []string{"check", "./docs/..."},
			wantCode:       ExitError,
			stderrContains: []string{`pattern "./docs/..." matches no adder.yaml [no-match]`},
		},
		{
			name:           "a directory without a configuration",
			args:           []string{"check", "./docs"},
			wantCode:       ExitError,
			stderrContains: []string{"[io]"},
		},
		{
			name:           "explain uses the configuration nearest the file",
			setup:          func(t *testing.T, root string) { run(t.Context(), "derive", "./...") },
			args:           []string{"explain", "services/api/cmds.md"},
			stdoutContains: []string{"services/api/cmds.md is generated by adder.\n", "  output    " + filepath.Join("services", "api", "adder.yaml") + ":6:7\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := monorepo(t)
			if tt.setup != nil {
				tt.setup(t, root)
			}
			code, stdout, stderr := run(t.Context(), tt.args...)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s", code, tt.wantCode, stdout, stderr)
			}
			if tt.wantStdout != "" && stdout != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tt.wantStdout)
			}
			for _, want := range tt.stdoutContains {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout = %q, want it to contain %q", stdout, want)
				}
			}
			for _, want := range tt.stderrContains {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr = %q, want it to contain %q", stderr, want)
				}
			}
		})
	}
}

func TestConfigsJSON(t *testing.T) {
	root := monorepo(t)
	writeFile(t, filepath.Join(root, "services", "web", "adder.yaml"), "version: 1\nderivee: {}\n")
	code, stdout, stderr := run(t.Context(), "check", "./...", "--json")
	if code != ExitError || stderr != "" {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	var got struct {
		Configs []struct {
			Config   string `json:"config"`
			Changes  []struct{ Path, Status string }
			Problems []struct{ Code string }
		} `json:"configs"`
		Problems []any `json:"problems"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout)
	}
	var configs, codes []string
	for _, c := range got.Configs {
		configs = append(configs, c.Config)
		for _, p := range c.Problems {
			codes = append(codes, c.Config+" "+p.Code)
		}
	}
	if want := []string{"adder.yaml", "services/api/adder.yaml", "services/web/adder.yaml"}; !slices.Equal(configs, want) {
		t.Errorf("configs = %v, want %v", configs, want)
	}
	if want := []string{"services/web/adder.yaml unknown-field"}; !slices.Equal(codes, want) {
		t.Errorf("problems = %v, want %v", codes, want)
	}
	if got.Problems == nil || len(got.Problems) != 0 {
		t.Errorf("top-level problems = %v, want an empty list", got.Problems)
	}
	if len(got.Configs[1].Changes) != 1 || got.Configs[1].Changes[0].Path != "cmds.md" {
		t.Errorf("changes = %+v, want cmds.md relative to its configuration", got.Configs[1].Changes)
	}
}

func TestFindConfigs(t *testing.T) {
	monorepo(t)
	tests := []struct {
		name     string
		patterns []string
		want     []string
		wantErr  bool
	}{
		{name: "everything, skipping testdata and hidden directories", patterns: []string{"./..."}, want: []string{"adder.yaml", "services/api/adder.yaml", "services/web/adder.yaml"}},
		{name: "a bare ...", patterns: []string{"..."}, want: []string{"adder.yaml", "services/api/adder.yaml", "services/web/adder.yaml"}},
		{name: "overlapping patterns", patterns: []string{"services/...", "./services/api"}, want: []string{"services/api/adder.yaml", "services/web/adder.yaml"}},
		{name: "the current directory", patterns: []string{"."}, want: []string{"adder.yaml"}},
		{name: "a missing directory", patterns: []string{"./missing/..."}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := findConfigs(tt.patterns)
			if (err != nil) != tt.wantErr {
				t.Fatalf("findConfigs() error = %v", err)
			}
			var got []string
			for _, f := range files {
				got = append(got, filepath.ToSlash(f))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("findConfigs() = %v, want %v", got, tt.want)
			}
		})
	}
}
