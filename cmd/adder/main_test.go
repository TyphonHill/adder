package main

import (
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

var update = flag.Bool("update", false, "rewrite the expected output in test scripts")

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){"adder": main})
}

func TestScripts(t *testing.T) {
	goEnv := goEnvironment(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		// Type-aware scans and a repository's own build run the go command, which needs the caches of the go
		// running the tests. Builds replace adder's module with this checkout, at ADDER_ROOT.
		Setup: func(env *testscript.Env) error {
			for key, value := range goEnv {
				env.Setenv(key, value)
			}
			env.Setenv("ADDER_ROOT", root)
			return nil
		},
		UpdateScripts:       *update,
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		Cmds:                map[string]func(*testscript.TestScript, bool, []string){"exits": exits},
	})
}

// exits runs a command and fails unless it exits with the given status: exits 3 adder check.
func exits(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("unsupported: ! exits")
	}
	if len(args) < 2 {
		ts.Fatalf("usage: exits status command [args...]")
	}
	want, err := strconv.Atoi(args[0])
	if err != nil {
		ts.Fatalf("invalid status %q", args[0])
	}
	got := 0
	if err := ts.Exec(args[1], args[2:]...); err != nil {
		exitErr, ok := errors.AsType[*exec.ExitError](err)
		if !ok {
			ts.Fatalf("%v", err)
		}
		got = exitErr.ExitCode()
	}
	if got != want {
		ts.Fatalf("%s exited with status %d, want %d", args[1], got, want)
	}
}

// goEnvironment returns the go command's cache and path settings, which scripts inherit.
func goEnvironment(t *testing.T) map[string]string {
	t.Helper()
	keys := []string{"GOCACHE", "GOMODCACHE", "GOPATH", "GOROOT"}
	out, err := exec.Command("go", append([]string{"env"}, keys...)...).Output()
	if err != nil {
		t.Fatalf("go env: %v", err)
	}
	env := map[string]string{"PATH": os.Getenv("PATH")}
	for i, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		env[keys[i]] = strings.TrimSpace(line)
	}
	return env
}
