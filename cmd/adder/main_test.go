package main

import (
	"errors"
	"flag"
	"os/exec"
	"strconv"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

var update = flag.Bool("update", false, "rewrite the expected output in test scripts")

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){"adder": main})
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:                 "testdata/script",
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
