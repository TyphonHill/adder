package adder

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"typhonhill.dev/adder/internal/cli"
)

// TestExamples checks that each example's derived files are current, that its generator tests pass, and that it
// builds, with adder from this checkout, so the examples always match the adder they document.
func TestExamples(t *testing.T) {
	tests := []struct {
		name string
		own  bool // the example builds its own adder, run with go tool adder
	}{
		{name: "registry"},
		{name: "handlers"},
		{name: "packs", own: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join("examples", tt.name)
			for _, command := range []string{"check", "test"} {
				if tt.own {
					run(t, dir, "go", "tool", "adder", command)
					continue
				}
				var stdout, stderr bytes.Buffer
				if code := cli.Main(t.Context(), "(devel)", nil, []string{command, "--root", dir}, strings.NewReader(""), &stdout, &stderr); code != cli.ExitOK {
					t.Errorf("adder %s exited with %d\n%s%s", command, code, stdout.String(), stderr.String())
				}
			}
			run(t, dir, "go", "build", "./...")
			run(t, dir, "go", "vet", "./...")
		})
	}
}

// run runs a command in dir with the go that runs the tests, never changing go.mod. In packs' workspace, adder
// is a main module, so the go command may download the go.mod files of adder's test-only dependencies.
func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOFLAGS=-mod=readonly")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}
