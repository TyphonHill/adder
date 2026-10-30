package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscover(t *testing.T) {
	registry := filepath.Join("drivers", "drivers.gen.go")
	derived := func(t *testing.T, root string) {
		if code, _, errOut := run(t.Context(), "--root", root, "derive"); code != ExitOK {
			t.Fatal(errOut)
		}
	}
	tests := []struct {
		name string
		// setup prepares the copied drivers fixture at root; it may be nil.
		setup          func(t *testing.T, root string)
		args           func(root string) []string
		chdir          string // relative to root
		wantCode       int
		wantStdout     string // exact, when set
		stdoutContains []string
		stderrContains []string
	}{
		{
			name:       "list",
			args:       func(root string) []string { return []string{"--root", root, "list"} },
			wantCode:   ExitOK,
			wantStdout: "generators\n  driver  Add a storage driver\noutputs\n  drivers/drivers.gen.go  from templates/drivers.go.tmpl\n",
		},
		{
			name: "list without generators or outputs",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "adder.yaml"), "version: 1\n")
			},
			args:       func(root string) []string { return []string{"--root", root, "list"} },
			wantCode:   ExitOK,
			wantStdout: "generators\n  none\noutputs\n  none\n",
		},
		{
			name: "list an output with each",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "adder.yaml"), "version: 1\nderive:\n  scans:\n    Drivers: {dirs: [\"drivers/*\"], funcs: {name: ^New$}}\n  outputs:\n    - {template: templates/new/README.md.tmpl, each: Drivers, path: \"{{ .Item.Dir }}/README.md\"}\n")
			},
			args:       func(root string) []string { return []string{"--root", root, "list"} },
			wantCode:   ExitOK,
			wantStdout: "generators\n  none\noutputs\n  {{ .Item.Dir }}/README.md  from templates/new/README.md.tmpl, one per Drivers item\n",
		},
		{
			name:     "list as JSON",
			args:     func(root string) []string { return []string{"--root", root, "list", "--json"} },
			wantCode: ExitOK,
			stdoutContains: []string{
				`"name": "driver",` + "\n" + `      "description": "Add a storage driver",`,
				`"template": "templates/drivers.go.tmpl",` + "\n" + `      "path": "drivers/drivers.gen.go",`,
				`"problems": []`,
			},
		},
		{
			name:     "describe",
			args:     func(root string) []string { return []string{"--root", root, "describe", "driver"} },
			wantCode: ExitOK,
			stdoutContains: []string{
				"driver: Add a storage driver\n\ninputs\n",
				"  --name     string  Driver name, used as the package name (required; must match [a-z][a-z0-9]*; not one of: local, s3)\n",
				`  --summary  string  One-line description for the package documentation (default "stores objects")` + "\n",
				"  drivers/{{ .Inputs.name }}/cache.go               from templates/new/cache.go.tmpl, with --cache\n",
				"  drivers/README.md                                 from templates/new/README.md.tmpl, skipped if it exists\n",
				"\nDerived outputs are updated in the same run.\n",
			},
		},
		{
			name: "describe values from a scan",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "adder.yaml"), `version: 1
derive:
  scans:
    Drivers: {dirs: ["drivers/*"], funcs: {name: ^New$}}
    None: {dirs: ["drivers/*"], funcs: {name: ^Nothing$}}
generators:
  copy:
    description: Copy a driver
    inputs:
      from: {valuesFrom: Drivers.Package, help: Driver to copy}
      into: {valuesFrom: None.Package}
    files:
      - {template: t.tmpl, path: x}
`)
			},
			args:     func(root string) []string { return []string{"--root", root, "describe", "copy"} },
			wantCode: ExitOK,
			stdoutContains: []string{
				"  --from  string  Driver to copy (required; one of: local, s3)\n",
				"  --into  string  (required; no value is allowed: None.Package has none)\n",
			},
		},
		{
			name: "describe a generator without inputs",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "adder.yaml"), "version: 1\ngenerators:\n  bare:\n    derive: false\n    inputs:\n      plain: {type: bool}\n    files:\n      - {template: t.tmpl, path: x, unless: plain}\n  none:\n    files: [{template: t.tmpl, path: y}]\n    derive: false\n")
			},
			args:       func(root string) []string { return []string{"--root", root, "describe", "none"} },
			wantCode:   ExitOK,
			wantStdout: "none: \n\ninputs\n  none\nfiles\n  y  from t.tmpl\n",
		},
		{
			name: "describe a file excluded by a bool input",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "adder.yaml"), "version: 1\ngenerators:\n  bare:\n    derive: false\n    inputs:\n      plain: {type: bool}\n    files:\n      - {template: t.tmpl, path: x, unless: plain}\n")
			},
			args:           func(root string) []string { return []string{"--root", root, "describe", "bare"} },
			wantCode:       ExitOK,
			stdoutContains: []string{"  --plain  bool  \n", "  x  from t.tmpl, without --plain\n"},
		},
		{
			name:     "describe as JSON includes the input schema",
			args:     func(root string) []string { return []string{"--root", root, "describe", "driver", "--json"} },
			wantCode: ExitOK,
			stdoutContains: []string{
				`"inputSchema": {` + "\n" + `    "$schema": "https://json-schema.org/draft/2020-12/schema",`,
				`"pattern": "^(?:[a-z][a-z0-9]*)$",` + "\n" + `        "not": {` + "\n" + `          "enum": [` + "\n" + `            "local",`,
				`"required": [` + "\n" + `      "name"` + "\n" + `    ],` + "\n" + `    "additionalProperties": false`,
			},
		},
		{
			name:           "describe an unknown generator",
			args:           func(root string) []string { return []string{"--root", root, "describe", "drivr"} },
			wantCode:       ExitUsage,
			stderrContains: []string{`unknown generator "drivr" [unknown-generator]`, "hint: did you mean driver?"},
		},
		{
			name: "describe with a failing scan",
			setup: func(t *testing.T, root string) {
				if err := os.RemoveAll(filepath.Join(root, "drivers")); err != nil {
					t.Fatal(err)
				}
			},
			args:           func(root string) []string { return []string{"--root", root, "describe", "driver"} },
			wantCode:       ExitError,
			stderrContains: []string{"[no-match]"},
		},
		{
			name:           "describe needs a generator",
			args:           func(root string) []string { return []string{"--root", root, "describe"} },
			wantCode:       ExitUsage,
			stderrContains: []string{"accepts 1 arg(s), received 0", "Run 'adder describe --help' for usage."},
		},
		{
			name:     "explain a derived output",
			setup:    derived,
			args:     func(root string) []string { return []string{"--root", root, "explain", filepath.Join(root, registry)} },
			wantCode: ExitOK,
			stdoutContains: []string{
				"drivers/drivers.gen.go is generated by adder.\n",
				"  template  templates/drivers.go.tmpl\n  scans     Drivers\n  status    up to date\n",
			},
		},
		{
			name: "explain from a subdirectory, an output that is out of date",
			setup: func(t *testing.T, root string) {
				derived(t, root)
				writeFile(t, filepath.Join(root, "drivers", "gcs", "gcs.go"), "package gcs\n\nfunc New() {}\n")
			},
			chdir:          "drivers",
			args:           func(string) []string { return []string{"explain", "drivers.gen.go"} },
			wantCode:       ExitOK,
			stdoutContains: []string{"  output    " + filepath.Join("..", "adder.yaml") + ":12:7\n", "  status    out of date; run adder derive to update it\n"},
		},
		{
			name: "explain when the plan does not render",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "adder.yaml"), "version: 1\nderive:\n  scans:\n    Drivers: {dirs: [\"drivers/*\"], funcs: {name: ^New$}}\n  outputs:\n    - {template: templates/new/cache.go.tmpl, each: Drivers, path: \"{{ .Item.Dir }}/x.gen.go\"}\n")
				writeFile(t, filepath.Join(root, "drivers", "gone", "x.gen.go"), "// Code generated by adder from templates/new/cache.go.tmpl. DO NOT EDIT.\n\npackage gone\n")
			},
			args: func(root string) []string {
				return []string{"--root", root, "explain", filepath.Join(root, "drivers", "gone", "x.gen.go")}
			},
			wantCode: ExitError,
			// The cache template reads .Inputs, which derive does not provide.
			stderrContains: []string{"[template-exec]"},
		},
		{
			name: "explain a file per item",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "adder.yaml"), "version: 1\nderive:\n  scans:\n    Drivers: {dirs: [\"drivers/*\"], funcs: {name: ^New$}}\n  outputs:\n    - {template: templates/new/README.md.tmpl, each: Drivers, path: \"{{ .Item.Dir }}/README.md\"}\n")
				derived(t, root)
			},
			args: func(root string) []string {
				return []string{"--root", root, "explain", filepath.Join(root, "drivers", "s3", "README.md")}
			},
			wantCode: ExitOK,
			stdoutContains: []string{
				"drivers/s3/README.md is generated by adder.\n",
				"  template  templates/new/README.md.tmpl\n  each      Drivers\n  scans     none\n  status    up to date\n",
			},
		},
		{
			name: "explain a file per item that is stale",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "adder.yaml"), "version: 1\nderive:\n  scans:\n    Drivers: {dirs: [\"drivers/*\"], funcs: {name: ^New$}}\n  outputs:\n    - {template: templates/new/README.md.tmpl, each: Drivers, path: \"{{ .Item.Dir }}/README.md\"}\n")
				writeFile(t, filepath.Join(root, "drivers", "gone", "README.md"), "<!-- Code generated by adder from templates/new/README.md.tmpl. DO NOT EDIT. -->\n")
			},
			args: func(root string) []string {
				return []string{"--root", root, "explain", filepath.Join(root, "drivers", "gone", "README.md")}
			},
			wantCode:       ExitOK,
			stdoutContains: []string{"  status    stale: no item produces it any more, and adder derive removes it\n"},
		},
		{
			name: "explain a file adder did not generate",
			args: func(root string) []string {
				return []string{"--root", root, "explain", filepath.Join(root, "drivers", "s3", "s3.go")}
			},
			wantCode:       ExitOK,
			stdoutContains: []string{"drivers/s3/s3.go was not generated by adder.\n"},
		},
		{
			name: "explain a file no output writes",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "old.gen.go"), "// Code generated by adder from templates/old.go.tmpl. DO NOT EDIT.\n\npackage old\n")
			},
			args: func(root string) []string {
				return []string{"--root", root, "explain", filepath.Join(root, "old.gen.go")}
			},
			wantCode: ExitOK,
			stdoutContains: []string{"old.gen.go was generated by adder from templates/old.go.tmpl, but no output in adder.yaml writes it.\n" +
				"It may be left over from an output that was removed, and can be deleted.\n"},
		},
		{
			name:  "explain as JSON",
			setup: derived,
			args: func(root string) []string {
				return []string{"--root", root, "explain", filepath.Join(root, registry), "--json"}
			},
			wantCode:       ExitOK,
			stdoutContains: []string{`"generated": true,`, `"scans": [` + "\n" + `    "Drivers"` + "\n  ],", `"status": "unchanged",`},
		},
		{
			name: "explain a file outside the root",
			args: func(root string) []string {
				return []string{"--root", root, "explain", filepath.Join(root, "..", "elsewhere.go")}
			},
			wantCode:       ExitError,
			stderrContains: []string{"file is outside the repository root", "[unsafe-path]"},
		},
		{
			name: "explain a missing file",
			args: func(root string) []string {
				return []string{"--root", root, "explain", filepath.Join(root, "missing.go")}
			},
			wantCode:       ExitError,
			stderrContains: []string{"missing.go", "[io]"},
		},
		{
			name:           "list without a configuration",
			args:           func(root string) []string { return []string{"--root", filepath.Join(root, "empty"), "list"} },
			wantCode:       ExitError,
			stderrContains: []string{"[io]"},
		},
		{
			name: "describe without a configuration",
			args: func(root string) []string {
				return []string{"--root", filepath.Join(root, "empty"), "describe", "driver"}
			},
			wantCode:       ExitError,
			stderrContains: []string{"[io]"},
		},
		{
			name:           "explain without a configuration",
			args:           func(root string) []string { return []string{"--root", filepath.Join(root, "empty"), "explain", "x.go"} },
			wantCode:       ExitError,
			stderrContains: []string{"[io]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyDir(t, filepath.Join("..", "scaffold", "testdata", "drivers", "repo"))
			if tt.setup != nil {
				tt.setup(t, root)
			}
			if tt.chdir != "" {
				t.Chdir(filepath.Join(root, tt.chdir))
			}

			code, stdout, stderr := run(t.Context(), tt.args(root)...)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d\nstderr: %s", code, tt.wantCode, stderr)
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
