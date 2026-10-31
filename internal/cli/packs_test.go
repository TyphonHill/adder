package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"typhonhill.dev/adder/internal/pack"
)

func TestPackFuncs(t *testing.T) {
	packs := []pack.Pack{{Name: "local", Funcs: map[string]any{"upper": strings.ToUpper}}}
	tests := []struct {
		name           string
		args           []string
		wantCode       int
		stdoutContains string
	}{
		{name: "derive renders paths and content", args: []string{"derive"}, stdoutContains: "created    OUT.txt\ncreated    notes/A.MD.yaml\n"},
		{name: "check", args: []string{"check"}},
		{name: "explain", args: []string{"explain", "notes/A.MD.yaml"}, stdoutContains: "up to date"},
		{name: "new renders paths and messages", args: []string{"new", "greet", "name=gcs"}, stdoutContains: "created    greet/GCS.txt\n\nSay GCS.\n"},
		{name: "test", args: []string{"test"}, stdoutContains: "ok    greet/plain\n"},
		{name: "describe", args: []string{"describe", "greet"}, stdoutContains: "greet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{
				"adder.yaml": `version: 1
derive:
  scans:
    Notes: {dirs: [notes], files: {name: '[.]md$'}}
  outputs:
    - {template: out.tmpl, path: OUT.txt}
    - {template: note.tmpl, each: Notes, path: 'notes/{{ upper .Item.Name }}.yaml'}
generators:
  greet:
    inputs:
      name: {pattern: "[a-z]+"}
    files:
      - {template: greet.tmpl, path: "greet/{{ upper .Inputs.name }}.txt"}
    message: Say {{ upper .Inputs.name }}.
    tests:
      - {name: plain, inputs: {name: example}, want: testdata/plain}
`,
				"notes/a.md":                       "a note\n",
				"note.tmpl":                        "name: {{ upper .Item.Name }}\n",
				"out.tmpl":                         `{{ upper "derived" }}` + "\n",
				"greet.tmpl":                       "hello {{ upper .Inputs.name }}\n",
				"testdata/plain/greet/EXAMPLE.txt": "hello EXAMPLE\n",
			}
			for name, content := range files {
				writeFile(t, filepath.Join(root, filepath.FromSlash(name)), content)
			}
			t.Chdir(root)
			if tt.args[0] != "derive" {
				if code, _, stderr := runMain(t.Context(), packs, "", "derive"); code != ExitOK {
					t.Fatalf("derive: exit code %d\n%s", code, stderr)
				}
			}
			code, stdout, stderr := runMain(t.Context(), packs, "", tt.args...)
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d\nstdout: %s\nstderr: %s", code, tt.wantCode, stdout, stderr)
			}
			if !strings.Contains(stdout, tt.stdoutContains) {
				t.Errorf("stdout = %q, want it to contain %q", stdout, tt.stdoutContains)
			}
		})
	}
}

func TestInvalidPack(t *testing.T) {
	packs := []pack.Pack{{Name: "local", Funcs: map[string]any{"join": strings.Join}}}
	tests := []struct {
		name       string
		args       []string
		wantStdout string
		wantStderr string
	}{
		{name: "text", args: []string{"--version"}, wantStderr: "adder: pack \"local\": function \"join\" is built in [invalid-pack]\n"},
		{name: "json", args: []string{"list", "--json"}, wantStdout: `"code": "invalid-pack"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runMain(t.Context(), packs, "", tt.args...)
			if code != ExitError {
				t.Errorf("exit code = %d, want %d", code, ExitError)
			}
			if !strings.Contains(stdout, tt.wantStdout) || !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("stdout = %q, stderr = %q, want %q and %q", stdout, stderr, tt.wantStdout, tt.wantStderr)
			}
		})
	}
}
