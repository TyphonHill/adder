package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	gcs := filepath.Join("drivers", "gcs", "gcs.go")
	const created = "created    drivers/gcs/gcs.go\ncreated    drivers/README.md\ncreated    drivers/drivers.gen.go\n\nNext, implement New in drivers/gcs/gcs.go.\n"
	tests := []struct {
		name string
		// setup prepares the copied fixture at root; it may be nil.
		setup          func(t *testing.T, root string)
		args           func(root string) []string
		stdin          string
		wantCode       int
		wantStdout     string // exact, when set
		stdoutContains []string
		stdoutExcludes []string
		stderrContains []string
		wantFile       string // a file that must exist afterwards, relative to the root
		wantNoFile     string // a file that must not exist afterwards, relative to the root
	}{
		{
			name:       "creates files and derived outputs",
			args:       func(root string) []string { return []string{"--root", root, "new", "driver", "--name", "gcs"} },
			wantCode:   ExitOK,
			wantStdout: created,
			wantFile:   gcs,
		},
		{
			name:       "global flags after new",
			args:       func(root string) []string { return []string{"new", "--root", root, "driver", "--name=gcs"} },
			wantCode:   ExitOK,
			wantStdout: created,
			wantFile:   gcs,
		},
		{
			name: "dry run",
			args: func(root string) []string {
				return []string{"--root", root, "new", "driver", "--name", "gcs", "--cache", "-n"}
			},
			wantCode: ExitOK,
			wantStdout: "would be created    drivers/gcs/gcs.go\nwould be created    drivers/gcs/cache.go\n" +
				"would be created    drivers/README.md\nwould be created    drivers/drivers.gen.go\n",
			wantNoFile: gcs,
		},
		{
			name: "json reports the generator, changes, and message",
			args: func(root string) []string {
				return []string{"--root", root, "new", "driver", "--name", "gcs", "--json"}
			},
			wantCode: ExitOK,
			stdoutContains: []string{
				`"generator": "driver",` + "\n" + `  "dryRun": false,`,
				`"path": "drivers/gcs/gcs.go",` + "\n" + `      "status": "created"`,
				`"message": "Next, implement New in drivers/gcs/gcs.go.",` + "\n" + `  "problems": []`,
			},
			stdoutExcludes: []string{"created    "},
			wantFile:       gcs,
		},
		{
			name: "diff previews every change as a patch",
			args: func(root string) []string {
				return []string{"--root", root, "new", "driver", "--name", "gcs", "--diff"}
			},
			wantCode: ExitOK,
			stdoutContains: []string{
				"would be created    drivers/gcs/gcs.go\n",
				"\ndiff /dev/null b/drivers/gcs/gcs.go\n--- /dev/null\n+++ b/drivers/gcs/gcs.go\n@@ -0,0 +1,7 @@\n+// Package gcs",
				"\ndiff /dev/null b/drivers/drivers.gen.go\n",
				"+\t\"gcs\":   gcs.New,\n",
			},
			stdoutExcludes: []string{"README.md\n--- a/", "Next, implement"},
			wantNoFile:     gcs,
		},
		{
			name: "diff leaves skipped files out",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "drivers", "README.md"), "mine\n")
			},
			args: func(root string) []string {
				return []string{"--root", root, "new", "driver", "--name", "gcs", "--diff"}
			},
			wantCode:       ExitOK,
			stdoutContains: []string{"skipped    drivers/README.md\n"},
			stdoutExcludes: []string{"b/drivers/README.md"},
		},
		{
			name: "skipped files",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "drivers", "README.md"), "mine\n")
			},
			args:           func(root string) []string { return []string{"--root", root, "new", "driver", "--name", "gcs"} },
			wantCode:       ExitOK,
			stdoutContains: []string{"skipped    drivers/README.md\n"},
		},
		{
			name: "inputs from standard input, overridden by flags",
			args: func(root string) []string {
				return []string{"--root", root, "new", "driver", "--inputs", "-", "--name", "gcs"}
			},
			stdin:    `{"name": "ignored", "cache": true}`,
			wantCode: ExitOK,
			wantFile: filepath.Join("drivers", "gcs", "cache.go"),
		},
		{
			name: "inputs from a file",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "inputs.json"), `{"name": "gcs"}`)
			},
			args: func(root string) []string {
				return []string{"--root", root, "new", "driver", "--inputs", filepath.Join(root, "inputs.json")}
			},
			wantCode: ExitOK,
			wantFile: gcs,
		},
		{
			name: "missing inputs file",
			args: func(root string) []string {
				return []string{"--root", root, "new", "driver", "--inputs", filepath.Join(root, "none.json")}
			},
			wantCode:       ExitError,
			stderrContains: []string{"none.json", "[io]"},
		},
		{
			name:           "inputs that are not an object",
			args:           func(root string) []string { return []string{"--root", root, "new", "driver", "--inputs", "-"} },
			stdin:          "null",
			wantCode:       ExitUsage,
			stderrContains: []string{"adder: inputs must be a JSON object of input names and values [invalid-input]"},
		},
		{
			name:           "lists generators",
			args:           func(root string) []string { return []string{"--root", root, "new"} },
			wantCode:       ExitOK,
			stdoutContains: []string{"new creates files from a generator", "driver      Add a storage driver"},
		},
		{
			name:     "generator help",
			args:     func(root string) []string { return []string{"--root", root, "new", "driver", "--help"} },
			wantCode: ExitOK,
			stdoutContains: []string{
				"adder new driver [flags]",
				"--name string      Driver name, used as the package name (required; not already in Drivers.Package)",
				`(default "stores objects")`,
			},
		},
		{
			name: "help when the configuration does not load",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "adder.yaml"), "version: 2\n")
			},
			args:           func(root string) []string { return []string{"--root", root, "new", "--help"} },
			wantCode:       ExitOK,
			stdoutContains: []string{"adder new <generator> [flags]"},
		},
		{
			name: "configuration problems",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "adder.yaml"), "version: 2\n")
			},
			args:           func(root string) []string { return []string{"--root", root, "new", "driver"} },
			wantCode:       ExitError,
			stderrContains: []string{"[unsupported-version]"},
		},
		{
			name:           "unknown generator",
			args:           func(root string) []string { return []string{"--root", root, "new", "drivr", "--name", "gcs"} },
			wantCode:       ExitUsage,
			stderrContains: []string{`adder: unknown generator "drivr" [unknown-generator]`, "hint: did you mean driver?"},
		},
		{
			name:           "missing input",
			args:           func(root string) []string { return []string{"--root", root, "new", "driver"} },
			wantCode:       ExitUsage,
			stderrContains: []string{"adder: --name is required [missing-input]"},
		},
		{
			name:           "invalid input",
			args:           func(root string) []string { return []string{"--root", root, "new", "driver", "--name", "s3"} },
			wantCode:       ExitUsage,
			stderrContains: []string{`--name "s3" already exists in Drivers.Package [invalid-input]`},
		},
		{
			name:           "unknown flag",
			args:           func(root string) []string { return []string{"--root", root, "new", "driver", "--nam", "gcs"} },
			wantCode:       ExitUsage,
			stderrContains: []string{"unknown flag: --nam", "Run 'adder new driver --help' for usage."},
		},
		{
			name:           "unexpected argument",
			args:           func(root string) []string { return []string{"--root", root, "new", "driver", "gcs"} },
			wantCode:       ExitUsage,
			stderrContains: []string{"Run 'adder new driver --help' for usage."},
		},
		{
			name: "existing file",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "drivers", "gcs", "gcs.go"), "package gcs\n")
			},
			args:           func(root string) []string { return []string{"--root", root, "new", "driver", "--name", "gcs"} },
			wantCode:       ExitError,
			stderrContains: []string{"gcs.go: file already exists [file-exists]"},
			wantNoFile:     filepath.Join("drivers", "README.md"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyDir(t, filepath.Join("..", "scaffold", "testdata", "drivers", "repo"))
			if tt.setup != nil {
				tt.setup(t, root)
			}

			code, stdout, stderr := runWithInput(t.Context(), tt.stdin, tt.args(root)...)

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
			for _, unwanted := range tt.stdoutExcludes {
				if strings.Contains(stdout, unwanted) {
					t.Errorf("stdout = %q, want it not to contain %q", stdout, unwanted)
				}
			}
			for _, want := range tt.stderrContains {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr = %q, want it to contain %q", stderr, want)
				}
			}
			if tt.wantFile != "" && !exists(t, filepath.Join(root, tt.wantFile)) {
				t.Errorf("%s was not created", tt.wantFile)
			}
			if tt.wantNoFile != "" && exists(t, filepath.Join(root, tt.wantNoFile)) {
				t.Errorf("%s was created", tt.wantNoFile)
			}
		})
	}
}
