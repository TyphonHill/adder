package adder

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"typhonhill.dev/adder/internal/cli"
)

// TestExamples checks, for every adder.yaml in each example, that its derived files are current and that its
// generator tests pass, and that every module in the example builds, with adder from this checkout, so the
// examples always match the adder they document.
func TestExamples(t *testing.T) {
	entries, err := os.ReadDir("examples")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			configs, modules := find(t, filepath.Join("examples", entry.Name()))
			if len(configs) == 0 || len(modules) == 0 {
				t.Fatalf("want an adder.yaml and a go.mod, found %d and %d", len(configs), len(modules))
			}
			for _, dir := range configs {
				for _, command := range []string{"check", "test"} {
					// An example with tools/adder builds its own adder, with its packs, and runs it with go tool adder.
					if _, err := os.Stat(filepath.Join(dir, "tools", "adder")); err == nil {
						run(t, dir, "go", "tool", "adder", command)
						continue
					}
					var stdout, stderr bytes.Buffer
					if code := cli.Main(t.Context(), "(devel)", nil, []string{command, "--root", dir}, strings.NewReader(""), &stdout, &stderr); code != cli.ExitOK {
						t.Errorf("adder %s in %s exited with %d\n%s%s", command, dir, code, stdout.String(), stderr.String())
					}
				}
			}
			for _, dir := range modules {
				run(t, dir, "go", "build", "./...")
				run(t, dir, "go", "vet", "./...")
			}
		})
	}
}

// find returns the directories under root holding an adder.yaml and those holding a go.mod, outside testdata.
func find(t *testing.T, root string) (configs, modules []string) {
	t.Helper()
	err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && d.Name() == "testdata":
			return filepath.SkipDir
		case d.Name() == "adder.yaml":
			configs = append(configs, filepath.Dir(name))
		case d.Name() == "go.mod":
			modules = append(modules, filepath.Dir(name))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return configs, modules
}

// run runs a command in dir with the go that runs the tests, never changing go.mod. In the workspace of an
// example that builds its own adder, adder is a main module, so the go command may download the go.mod files of
// adder's test-only dependencies.
func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOFLAGS=-mod=readonly")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}
