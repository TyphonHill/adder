package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTestCommand(t *testing.T) {
	const golden = "testdata/generators/driver/plain"
	withTests := func(t *testing.T, root string) {
		config, err := os.ReadFile(filepath.Join(root, "adder.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(root, "adder.yaml"), string(config)+"    tests:\n      - {name: plain, inputs: {name: example}, want: "+golden+"}\n")
	}
	accepted := func(t *testing.T, root string) {
		withTests(t, root)
		if code, _, errOut := run(t.Context(), "--root", root, "test", "--update"); code != ExitOK {
			t.Fatal(errOut)
		}
	}
	tests := []struct {
		name           string
		setup          func(t *testing.T, root string)
		args           []string
		wantCode       int
		wantStdout     string // exact, when set
		stdoutContains []string
		stderrContains []string
		wantGolden     bool // the golden files exist afterwards
	}{
		{
			name:           "a test without golden files fails with a diff",
			setup:          withTests,
			args:           []string{"test"},
			wantCode:       ExitError,
			stdoutContains: []string{"FAIL  driver/plain\n", "diff /dev/null b/" + golden + "/drivers/example/example.go\n"},
			stderrContains: []string{"test driver/plain: the generator's files differ from " + golden + " [golden-mismatch]", "hint: run adder test --update"},
		},
		{
			name:  "update writes the golden files",
			setup: withTests,
			args:  []string{"test", "--update"},
			wantStdout: "created    " + golden + "/drivers/README.md\n" +
				"created    " + golden + "/drivers/example/example.go\n",
			wantGolden: true,
		},
		{
			name:       "a test that matches",
			setup:      accepted,
			args:       []string{"test", "driver"},
			wantStdout: "ok    driver/plain\n",
			wantGolden: true,
		},
		{
			name:       "update with nothing to change",
			setup:      accepted,
			args:       []string{"test", "--update"},
			wantStdout: "golden files are up to date\n",
			wantGolden: true,
		},
		{
			name:           "as JSON",
			setup:          withTests,
			args:           []string{"test", "--json"},
			wantCode:       ExitError,
			stdoutContains: []string{`"generator": "driver",` + "\n" + `      "name": "plain",` + "\n" + `      "passed": false,`, `"code": "golden-mismatch"`},
		},
		{
			name:       "no tests",
			args:       []string{"test"},
			wantStdout: "no generator tests\n",
		},
		{
			name:           "an unknown generator",
			args:           []string{"test", "drivr"},
			wantCode:       ExitUsage,
			stderrContains: []string{`unknown generator "drivr"`, "did you mean driver?"},
		},
		{
			name: "inputs the generator rejects",
			setup: func(t *testing.T, root string) {
				config, err := os.ReadFile(filepath.Join(root, "adder.yaml"))
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(root, "adder.yaml"), string(config)+"    tests:\n      - {name: taken, inputs: {name: s3}, want: w}\n")
			},
			args:           []string{"test"},
			wantCode:       ExitUsage,
			stderrContains: []string{`input name "s3" already exists in Drivers.Package [invalid-input]`},
		},
		{
			name: "without a configuration",
			setup: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "adder.yaml")); err != nil {
					t.Fatal(err)
				}
			},
			args:           []string{"test"},
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

			code, stdout, stderr := run(t.Context(), append([]string{"--root", root}, tt.args...)...)

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
			if got := exists(t, filepath.Join(root, filepath.FromSlash(golden), "drivers", "example", "example.go")); got != tt.wantGolden {
				t.Errorf("golden file exists = %v, want %v", got, tt.wantGolden)
			}
		})
	}
}
