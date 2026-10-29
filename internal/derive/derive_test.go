package derive

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/diag"
	"github.com/TyphonHill/adder/internal/plan"
)

var update = flag.Bool("update", false, "rewrite golden files")

// output is where the fixture's configuration writes its registry.
const output = "drivers/drivers.gen.go"

// copyRepo copies testdata/<fixture>/repo into a temporary directory and loads its configuration.
func copyRepo(t *testing.T, fixture string) (string, *config.Config) {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("testdata", fixture, "repo"))); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	return root, cfg
}

func statuses(p *plan.Plan) []plan.Status {
	if p == nil {
		return nil
	}
	var got []plan.Status
	for _, c := range p.Changes {
		got = append(got, c.Status)
	}
	return got
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return err == nil
}

func TestRunGolden(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		outputs []string
	}{
		{name: "driver registry", fixture: "basic", outputs: []string{output}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, cfg := copyRepo(t, tt.fixture)
			if _, err := Run(t.Context(), cfg, Options{Root: root}); err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			for _, out := range tt.outputs {
				got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(out)))
				if err != nil {
					t.Fatal(err)
				}
				golden := filepath.Join("testdata", tt.fixture, "golden", filepath.Base(out))
				if *update {
					if err := os.WriteFile(golden, got, 0o644); err != nil {
						t.Fatal(err)
					}
				}
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != string(want) {
					t.Errorf("%s differs from %s (run go test -update to accept):\ngot:\n%s\nwant:\n%s", out, golden, got, want)
				}
			}
		})
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name string
		// setup changes the copied fixture before Run; it may be nil.
		setup func(t *testing.T, root string, cfg *config.Config)
		// options returns the options for Run; nil means Options{Root: root}.
		options      func(t *testing.T, root string) Options
		wantStatuses []plan.Status
		wantCodes    []diag.Code
		wantHint     string
		wantFiles    []string // diagnostic files, matched by suffix
		wantInRoot   bool     // the output exists in the root afterwards
	}{
		{
			name:         "creates outputs",
			wantStatuses: []plan.Status{plan.Created},
			wantInRoot:   true,
		},
		{
			name: "leaves current outputs unchanged",
			setup: func(t *testing.T, root string, cfg *config.Config) {
				if _, err := Run(t.Context(), cfg, Options{Root: root}); err != nil {
					t.Fatal(err)
				}
			},
			wantStatuses: []plan.Status{plan.Unchanged},
			wantInRoot:   true,
		},
		{
			name:         "dry run writes nothing",
			options:      func(_ *testing.T, root string) Options { return Options{Root: root, DryRun: true} },
			wantStatuses: []plan.Status{plan.Created},
		},
		{
			name: "writes to the out directory instead of the root",
			options: func(t *testing.T, root string) Options {
				return Options{Root: root, OutDir: filepath.Join(t.TempDir(), "preview")}
			},
			wantStatuses: []plan.Status{plan.Created},
		},
		{
			name: "missing root",
			options: func(t *testing.T, _ string) Options {
				return Options{Root: filepath.Join(t.TempDir(), "missing")}
			},
			wantCodes: []diag.Code{diag.IO},
		},
		{
			name: "without go.mod there is no Module field",
			setup: func(t *testing.T, root string, _ *config.Config) {
				if err := os.Remove(filepath.Join(root, "go.mod")); err != nil {
					t.Fatal(err)
				}
			},
			wantCodes: []diag.Code{diag.TemplateExec},
			wantHint:  "available fields: Drivers",
		},
		{
			name: "reports every output problem and writes nothing",
			setup: func(_ *testing.T, _ string, cfg *config.Config) {
				cfg.Derive.Outputs = append(cfg.Derive.Outputs,
					config.Output{Template: "templates/missing.tmpl", Path: "a.go"},
					config.Output{Template: "templates/also-missing.tmpl", Path: "b.go"},
				)
			},
			wantCodes: []diag.Code{diag.IO, diag.IO},
			wantFiles: []string{filepath.Join("templates", "missing.tmpl"), filepath.Join("templates", "also-missing.tmpl")},
		},
		{
			name: "attributes scan problems to the scan",
			setup: func(_ *testing.T, _ string, cfg *config.Config) {
				cfg.Derive.Scans["Plugins"] = config.Scan{Dirs: []string{"plugins/*"}, Pos: diag.Pos{File: "adder.yaml", Line: 9}}
			},
			wantCodes: []diag.Code{diag.NoMatch},
			wantFiles: []string{"adder.yaml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, cfg := copyRepo(t, "basic")
			if tt.setup != nil {
				tt.setup(t, root, cfg)
			}
			opts := Options{Root: root}
			if tt.options != nil {
				opts = tt.options(t, root)
			}

			p, err := Run(t.Context(), cfg, opts)

			list := diag.From(err)
			var codes []diag.Code
			for _, d := range list {
				codes = append(codes, d.Code)
			}
			if !slices.Equal(codes, tt.wantCodes) {
				t.Fatalf("Run() problems = %v, want %v\n%v", codes, tt.wantCodes, err)
			}
			for i, suffix := range tt.wantFiles {
				if !strings.HasSuffix(list[i].Pos.File, suffix) {
					t.Errorf("problem %d is in %q, want a file ending in %q", i, list[i].Pos.File, suffix)
				}
			}
			if tt.wantHint != "" && list[0].Hint != tt.wantHint {
				t.Errorf("Hint = %q, want %q", list[0].Hint, tt.wantHint)
			}
			if got := statuses(p); !slices.Equal(got, tt.wantStatuses) {
				t.Errorf("statuses = %v, want %v", got, tt.wantStatuses)
			}

			if got := exists(t, filepath.Join(root, filepath.FromSlash(output))); got != tt.wantInRoot {
				t.Errorf("output in root = %v, want %v", got, tt.wantInRoot)
			}
			if opts.OutDir != "" && !exists(t, filepath.Join(opts.OutDir, filepath.FromSlash(output))) {
				t.Errorf("output missing from %s", opts.OutDir)
			}
		})
	}
}

func TestPlan(t *testing.T) {
	repo := fstest.MapFS{
		"go.mod":       {Data: []byte("module example.com/m\n")},
		"cmds/a/a.go":  {Data: []byte("package a\n\nfunc Run() {}\n")},
		"cmds/b/b.go":  {Data: []byte("package b\n\nfunc Run() {}\n")},
		"cmds/list.go": {Data: []byte("not scanned")},
	}
	templates := fstest.MapFS{
		"list.tmpl": {Data: []byte("{{ range .Cmds }}{{ $.Module }}/{{ .Dir }}\n{{ end }}")},
	}
	const rendered = "example.com/m/cmds/a\nexample.com/m/cmds/b\n"
	cfg := &config.Config{Version: 1, Derive: config.Derive{
		Scans:   map[string]config.Scan{"Cmds": {Dirs: []string{"cmds/*"}}},
		Outputs: []config.Output{{Template: "list.tmpl", Path: "cmds.txt"}},
	}}

	tests := []struct {
		name     string
		existing fstest.MapFS
		want     plan.Status
	}{
		{name: "no existing outputs", existing: nil, want: plan.Created},
		{name: "current output", existing: fstest.MapFS{"cmds.txt": {Data: []byte(rendered)}}, want: plan.Unchanged},
		{name: "outdated output", existing: fstest.MapFS{"cmds.txt": {Data: []byte("old")}}, want: plan.Updated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := Sources{Repo: repo, Templates: templates}
			if tt.existing != nil {
				src.Existing = tt.existing
			}
			p, err := Plan(t.Context(), cfg, src)
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}
			if got := statuses(p); !slices.Equal(got, []plan.Status{tt.want}) {
				t.Errorf("statuses = %v, want [%v]", got, tt.want)
			}
			if got := string(p.Changes[0].New); got != rendered {
				t.Errorf("content = %q, want %q", got, rendered)
			}
		})
	}
}
