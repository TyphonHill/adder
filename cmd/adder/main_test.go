package main

import (
	"errors"
	"flag"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
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
			env.Setenv("ADDER_GOMODCACHE", goEnv["GOMODCACHE"])
			// Scripts run the go that runs the tests, never a toolchain go.mod asks for, which may not be downloaded.
			env.Setenv("GOTOOLCHAIN", "local")
			return nil
		},
		UpdateScripts:       *update,
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		Cmds:                map[string]func(*testscript.TestScript, bool, []string){"exits": exits, "modproxy": modproxy},
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

// modproxy publishes a module offline: modproxy example.com/m@v1.0.0 dir adds the module in dir to a proxy in
// $WORK/proxy. The go command then downloads from that proxy, then from the module cache of the go running the
// tests, into a module cache of the script's own, so no test module reaches the shared one.
func modproxy(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 2 {
		ts.Fatalf("usage: modproxy module@version dir")
	}
	path, version, ok := strings.Cut(args[0], "@")
	if !ok {
		ts.Fatalf("modproxy: %q has no version", args[0])
	}
	escaped, err := module.EscapePath(path)
	ts.Check(err)
	proxy := filepath.Join(ts.Getenv("WORK"), "proxy")
	dir := filepath.Join(proxy, filepath.FromSlash(escaped), "@v")
	ts.Check(os.MkdirAll(dir, 0o755))
	src := ts.MkAbs(args[1])

	gomod, err := os.ReadFile(filepath.Join(src, "go.mod"))
	ts.Check(err)
	ts.Check(os.WriteFile(filepath.Join(dir, version+".mod"), gomod, 0o644))
	ts.Check(os.WriteFile(filepath.Join(dir, version+".info"), []byte(`{"Version":"`+version+`","Time":"2026-01-01T00:00:00Z"}`), 0o644))
	zipFile, err := os.Create(filepath.Join(dir, version+".zip"))
	ts.Check(err)
	ts.Check(modzip.CreateFromDir(zipFile, module.Version{Path: path, Version: version}, src))
	ts.Check(zipFile.Close())
	list, err := os.OpenFile(filepath.Join(dir, "list"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	ts.Check(err)
	_, err = list.WriteString(version + "\n")
	ts.Check(err)
	ts.Check(list.Close())

	ts.Setenv("GOPROXY", fileURL(proxy)+","+fileURL(filepath.Join(ts.Getenv("ADDER_GOMODCACHE"), "cache", "download")))
	ts.Setenv("GOMODCACHE", filepath.Join(ts.Getenv("WORK"), "modcache"))
	ts.Setenv("GOFLAGS", "-modcacherw") // so the work directory can be removed
	ts.Setenv("GONOSUMDB", path)
}

// fileURL returns the file URL of an absolute directory, on Windows too.
func fileURL(dir string) string {
	return (&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(dir), "/")}).String()
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
