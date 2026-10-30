package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"golang.org/x/tools/go/packages"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/diag"
	"github.com/TyphonHill/adder/internal/header"
)

// typedModule writes a module for type-aware scans to a temporary directory.
func typedModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files["go.mod"] = "module example.com/m\n\ngo 1.26.0\n"
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func names(items []Item) []string {
	var out []string
	for _, it := range items {
		s := it.Name
		if it.Pointer {
			s = "*" + s
		}
		out = append(out, s)
	}
	return out
}

func TestRunTyped(t *testing.T) {
	root := typedModule(t, map[string]string{
		"driver/driver.go":         "package driver\n\ntype Driver interface{ Get(key string) ([]byte, error) }\n",
		"drivers/a/a.go":           "package a\n\ntype Value struct{}\n\nfunc (Value) Get(string) ([]byte, error) { return nil, nil }\n\ntype Pointer struct{}\n\nfunc (*Pointer) Get(string) ([]byte, error) { return nil, nil }\n",
		"drivers/a/not.go":         "package a\n\ntype Not struct{}\n\ntype Iface interface{ Get(key string) ([]byte, error) }\n\ntype Generic[T any] struct{}\n\nfunc (Generic[T]) Get(string) ([]byte, error) { return nil, nil }\n",
		"drivers/a/gen.go":         header.Format("t.tmpl") + "\n\npackage a\n\ntype Generated struct{}\n\nfunc (Generated) Get(string) ([]byte, error) { return helper() }\n",
		"drivers/a/use.go":         "package a\n\nfunc helper() ([]byte, error) { return Generated{}.Get(\"\") }\n",
		"drivers/linux/l.go":       "//go:build linux\n\npackage linux\n\ntype OnLinux struct{}\n\nfunc (OnLinux) Get(string) ([]byte, error) { return nil, nil }\n",
		"drivers/win/w_windows.go": "package win\n\ntype OnWindows struct{}\n\nfunc (OnWindows) Get(string) ([]byte, error) { return nil, nil }\n",
		"drivers/tagged/t.go":      "//go:build integration\n\npackage tagged\n\ntype Tagged struct{}\n\nfunc (Tagged) Get(string) ([]byte, error) { return nil, nil }\n",
		"drivers/tested/t.go":      "package tested\n",
		"drivers/tested/t_test.go": "package tested\n\ntype InTest struct{}\n\nfunc (InTest) Get(string) ([]byte, error) { return nil, nil }\n",
		"drivers/tested/x_test.go": "package tested_test\n\ntype External struct{}\n\nfunc (External) Get(string) ([]byte, error) { return nil, nil }\n",
		"drivers/empty/README.md":  "no Go here\n",
		"errs/e.go":                "package errs\n\ntype Fail struct{}\n\nfunc (Fail) Error() string { return \"\" }\n",
		"handlers/h.go": "package handlers\n\nimport (\n\tstd \"context\"\n\t. \"context\"\n)\n\ntype C = std.Context\n\n" +
			"func Aliased(c std.Context) {}\n\nfunc Dot(c Context) {}\n\nfunc ThroughAlias(c C) {}\n\nfunc Other(s string) {}\n\nfunc None() {}\n",
		"color/color.go": "package color\n\ntype Color int\n\nconst Red = Color(1)\n\nconst Blue Color = 2\n\nconst Size = 3\n",
	})
	drivers := func(s config.Scan) config.Scan {
		s.Dirs, s.TypeCheck = []string{"drivers/*"}, true
		s.Types = &config.TypeMatch{Implements: "example.com/m/driver.Driver"}
		return s
	}
	tests := []struct {
		name    string
		scan    config.Scan
		overlay map[string][]byte
		want    []string
	}{
		{name: "types that implement an interface", scan: drivers(config.Scan{}), want: []string{"OnLinux", "*Pointer", "Value"}},
		{name: "another platform", scan: drivers(config.Scan{Platform: "windows/amd64"}), want: []string{"OnWindows", "*Pointer", "Value"}},
		{name: "build tags", scan: drivers(config.Scan{BuildTags: []string{"integration"}}), want: []string{"OnLinux", "*Pointer", "Tagged", "Value"}},
		{name: "tests", scan: drivers(config.Scan{IncludeTests: true}), want: []string{"External", "InTest", "OnLinux", "*Pointer", "Value"}},
		{
			name:    "files that are not on disk yet",
			scan:    drivers(config.Scan{}),
			overlay: map[string][]byte{"drivers/b/b.go": []byte("package b\n\ntype New struct{}\n\nfunc (New) Get(string) ([]byte, error) { return nil, nil }\n")},
			want:    []string{"New", "OnLinux", "*Pointer", "Value"},
		},
		{
			name: "the error interface",
			scan: config.Scan{Dirs: []string{"errs"}, TypeCheck: true, Types: &config.TypeMatch{Implements: "error"}},
			want: []string{"Fail"},
		},
		{
			name: "a parameter type through import and type aliases",
			scan: config.Scan{Dirs: []string{"handlers"}, TypeCheck: true, Funcs: config.FuncMatch{FirstParamType: "context.Context"}},
			want: []string{"Aliased", "Dot", "ThroughAlias"},
		},
		{
			name: "a value's type, named locally",
			scan: config.Scan{Dirs: []string{"color"}, TypeCheck: true, Values: &config.ValueMatch{Type: "Color"}},
			want: []string{"Blue", "Red"},
		},
		{
			name: "a value's type, with its import path",
			scan: config.Scan{Dirs: []string{"color"}, TypeCheck: true, Values: &config.ValueMatch{Type: "example.com/m/color.Color"}},
			want: []string{"Blue", "Red"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RunTyped(t.Context(), TypedSource{FS: withOverlay(t, root, tt.overlay), Dir: root, Overlay: tt.overlay}, tt.scan)
			if err != nil {
				t.Fatalf("RunTyped() error = %v", err)
			}
			if !slices.Equal(names(got), tt.want) {
				t.Errorf("items = %v, want %v", names(got), tt.want)
			}
			for _, it := range got {
				if !strings.HasPrefix(it.ImportPath, "example.com/m/") || it.File == "" || it.Line == 0 {
					t.Errorf("item %s has import path %q, file %q, line %d", it.Name, it.ImportPath, it.File, it.Line)
				}
			}
		})
	}
}

// withOverlay returns the files under root with overlay laid over them, as adder new's file system shows them.
func withOverlay(t *testing.T, root string, overlay map[string][]byte) fs.FS {
	t.Helper()
	if overlay == nil {
		return os.DirFS(root)
	}
	files := fstest.MapFS{}
	err := fs.WalkDir(os.DirFS(root), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		files[name] = &fstest.MapFile{Data: data}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range overlay {
		files[name] = &fstest.MapFile{Data: data}
	}
	return files
}

func TestRunTypedProblems(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		scan     config.Scan
		noDir    bool
		wantFile string // the problem's file, when it has one
		wantLine int
		wantHint bool
	}{
		{
			name:     "code that does not type check",
			files:    map[string]string{"p/p.go": "package p\n\nfunc F() int { return \"x\" }\n"},
			scan:     config.Scan{Dirs: []string{"p"}, TypeCheck: true},
			wantFile: "p/p.go",
			wantLine: 3,
		},
		{
			name:     "a missing dependency",
			files:    map[string]string{"p/p.go": "package p\n\nimport _ \"example.org/missing/pkg\"\n"},
			scan:     config.Scan{Dirs: []string{"p"}, TypeCheck: true},
			wantFile: "p/p.go",
			wantHint: true,
		},
		{
			name:  "an interface that does not exist",
			files: map[string]string{"p/p.go": "package p\n\ntype T struct{}\n"},
			scan:  config.Scan{Dirs: []string{"p"}, TypeCheck: true, Types: &config.TypeMatch{Implements: "example.com/m/p.Nope"}},
		},
		{
			name:  "a type that is not an interface",
			files: map[string]string{"p/p.go": "package p\n\ntype T struct{}\n"},
			scan:  config.Scan{Dirs: []string{"p"}, TypeCheck: true, Types: &config.TypeMatch{Implements: "example.com/m/p.T"}},
		},
		{
			name:  "no directory on disk",
			files: map[string]string{"p/p.go": "package p\n"},
			scan:  config.Scan{Dirs: []string{"p"}, TypeCheck: true},
			noDir: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := typedModule(t, tt.files)
			src := TypedSource{FS: os.DirFS(root), Dir: root}
			if tt.noDir {
				src.Dir = ""
			}
			_, err := RunTyped(t.Context(), src, tt.scan)
			list := diag.From(err)
			if len(list) == 0 || list[0].Code != diag.TypeCheck {
				t.Fatalf("RunTyped() error = %v, want a type-check problem", err)
			}
			d := list[0]
			if d.Pos.File != tt.wantFile || (tt.wantLine != 0 && d.Pos.Line != tt.wantLine) {
				t.Errorf("problem at %v, want %s line %d: %s", d.Pos, tt.wantFile, tt.wantLine, d.Message)
			}
			if tt.wantHint && d.Hint == "" {
				t.Errorf("problem %q has no hint", d.Message)
			}
		})
	}
}

func TestRunTypedNothingToLoad(t *testing.T) {
	root := typedModule(t, map[string]string{"docs/README.md": "x", "only/t_test.go": "package only\n"})
	items, err := RunTyped(t.Context(), TypedSource{FS: os.DirFS(root), Dir: root}, config.Scan{Dirs: []string{"docs", "only"}, TypeCheck: true})
	if err != nil || items != nil {
		t.Errorf("RunTyped() = %v, %v, want nothing", items, err)
	}
}

func TestSplitPos(t *testing.T) {
	tests := []struct {
		pos       string
		file      string
		line, col int
	}{
		{pos: "/a/b.go:3:4", file: "/a/b.go", line: 3, col: 4},
		{pos: "/a/b.go:3", file: "/a/b.go", line: 3},
		{pos: `C:\a\b.go:3:4`, file: `C:\a\b.go`, line: 3, col: 4},
		{pos: "/a/b.go", file: "/a/b.go"},
		{pos: "-"},
		{pos: ""},
	}
	for _, tt := range tests {
		t.Run(tt.pos, func(t *testing.T) {
			file, line, col := splitPos(tt.pos)
			if file != tt.file || line != tt.line || col != tt.col {
				t.Errorf("splitPos(%q) = %q, %d, %d, want %q, %d, %d", tt.pos, file, line, col, tt.file, tt.line, tt.col)
			}
		})
	}
}

func TestGoEnv(t *testing.T) {
	t.Setenv("GOOS", "darwin")
	t.Setenv("GOFLAGS", "-tags=local")
	t.Setenv("GOPROXY", "https://proxy.golang.org")
	root := t.TempDir()
	env := goEnv(root, "")
	for _, want := range []string{"GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0", "GOFLAGS=-mod=readonly", "GOPROXY=off", "GOWORK=off"} {
		if !slices.Contains(env, want) {
			t.Errorf("environment lacks %s", want)
		}
	}
	for _, unwanted := range []string{"GOOS=darwin", "GOFLAGS=-tags=local", "GOPROXY=https://proxy.golang.org"} {
		if slices.Contains(env, unwanted) {
			t.Errorf("environment keeps %s", unwanted)
		}
	}

	for _, name := range []string{"go.work", filepath.Join("vendor", "modules.txt")} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	env = goEnv(root, "windows/arm64")
	for _, want := range []string{"GOOS=windows", "GOARCH=arm64", "GOFLAGS=-mod=vendor", "GOWORK=" + filepath.Join(root, "go.work")} {
		if !slices.Contains(env, want) {
			t.Errorf("environment lacks %s", want)
		}
	}
}

func TestLoadError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	tests := []struct {
		name     string
		err      packages.Error
		wantPos  diag.Pos
		wantMsg  string
		wantHint bool
	}{
		{
			name:    "compiler output",
			err:     packages.Error{Msg: "# example.com/m/p\np/p.go:3:23: cannot use x", Kind: packages.ListError},
			wantPos: diag.Pos{File: "p/p.go", Line: 3, Column: 23},
			wantMsg: "cannot use x",
		},
		{
			name:    "an absolute position in the root",
			err:     packages.Error{Pos: filepath.Join(root, "p", "p.go") + ":2:1", Msg: "bad"},
			wantPos: diag.Pos{File: "p/p.go", Line: 2, Column: 1},
			wantMsg: "bad",
		},
		{
			name:     "a relative position and a missing module",
			err:      packages.Error{Pos: "p/p.go:3:8", Msg: "cannot find module providing package example.org/x"},
			wantPos:  diag.Pos{File: "p/p.go", Line: 3, Column: 8},
			wantMsg:  "cannot find module providing package example.org/x",
			wantHint: true,
		},
		{
			name:    "a position outside the root",
			err:     packages.Error{Pos: filepath.Join(filepath.Dir(root), "other.go") + ":1:1", Msg: "bad"},
			wantPos: diag.Pos{File: filepath.ToSlash(filepath.Join(filepath.Dir(root), "other.go")), Line: 1, Column: 1},
			wantMsg: "bad",
		},
		{name: "no position", err: packages.Error{Msg: "# example.com/m/p\nno position here"}, wantMsg: "# example.com/m/p\nno position here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := loadError(root, tt.err)
			if d.Code != diag.TypeCheck || d.Pos != tt.wantPos || d.Message != tt.wantMsg || (d.Hint != "") != tt.wantHint {
				t.Errorf("loadError() = %+v, want %v %q (hint %v)", d, tt.wantPos, tt.wantMsg, tt.wantHint)
			}
		})
	}
}
