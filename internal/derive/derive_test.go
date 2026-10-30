package derive

import (
	"cmp"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/diag"
	"github.com/TyphonHill/adder/internal/header"
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
			wantHint:  "available fields: Drivers, Vars",
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

func TestPlanShared(t *testing.T) {
	repo := fstest.MapFS{
		"cmds/a/a.go": {Data: []byte("package a\n\nfunc Run() {}\n")},
	}
	templates := fstest.MapFS{
		"list.tmpl":                  {Data: []byte(`{{ template "header" . }}{{ range .Cmds }}{{ .Package }}{{ end }}`)},
		"vars.tmpl":                  {Data: []byte(`[{{ range $k, $v := .Vars }}{{ $k }}={{ $v }} {{ end }}]`)},
		"partials/header.tmpl":       {Data: []byte(`{{ define "header" }}// {{ .Vars.License }}{{ "\n" }}{{ end }}`)},
		"partials/also/header.tmpl":  {Data: []byte(`{{ define "header" }}other{{ end }}`)},
		"partials/broken/x.tmpl":     {Data: []byte(`{{ define "x" }}`)},
		"partials/dir.tmpl/x.tmpl":   {Data: []byte(``)},
		"partials/dironly/sub/x.txt": {Data: []byte(``)},
	}
	at := diag.Pos{File: "adder.yaml", Line: 3}

	tests := []struct {
		name      string
		vars      map[string]string
		partials  []string
		template  string
		scanDirs  []string // replaces the scan's dirs when set
		want      string
		wantCodes []diag.Code
		wantFiles []string // diagnostic files, matched by suffix
	}{
		{name: "vars and partials", vars: map[string]string{"License": "MIT"}, partials: []string{"partials/*.tmpl"}, template: "list.tmpl", want: "// MIT\na"},
		{name: "vars are empty when none are set", template: "vars.tmpl", want: "[]"},
		{name: "vars", vars: map[string]string{"B": "2", "A": "1"}, template: "vars.tmpl", want: "[A=1 B=2 ]"},
		{
			name:      "a pattern matching only directories matches nothing",
			partials:  []string{"partials/dironly/*"},
			template:  "vars.tmpl",
			wantCodes: []diag.Code{diag.NoMatch},
			wantFiles: []string{"adder.yaml"},
		},
		{
			name:     "files matched by several patterns load once",
			vars:     map[string]string{"License": "MIT"},
			partials: []string{"partials/*.tmpl", "partials/header.tmpl"},
			template: "list.tmpl",
			want:     "// MIT\na",
		},
		{
			name:      "partials defining the same template",
			partials:  []string{"partials/header.tmpl", "partials/also/*.tmpl"},
			template:  "list.tmpl",
			wantCodes: []diag.Code{diag.TemplateSyntax},
			wantFiles: []string{filepath.Join("tmpl", "partials", "header.tmpl")},
		},
		{
			name:      "partial and scan problems are reported together",
			partials:  []string{"partials/missing/*.tmpl", "partials/broken/*.tmpl"},
			template:  "list.tmpl",
			scanDirs:  []string{"missing/*"},
			wantCodes: []diag.Code{diag.NoMatch, diag.TemplateSyntax, diag.NoMatch},
			wantFiles: []string{"adder.yaml", filepath.Join("tmpl", "partials", "broken", "x.tmpl"), "adder.yaml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scan := config.Scan{Dirs: []string{"cmds/*"}, Pos: at}
			if tt.scanDirs != nil {
				scan.Dirs = tt.scanDirs
			}
			cfg := &config.Config{Version: 1, Vars: tt.vars, Derive: config.Derive{
				Scans:   map[string]config.Scan{"Cmds": scan},
				Outputs: []config.Output{{Template: tt.template, Path: "out.txt", Pos: at}},
			}}
			for _, pattern := range tt.partials {
				cfg.Partials = append(cfg.Partials, config.Partial{Pattern: pattern, Pos: at})
			}

			p, err := Plan(t.Context(), cfg, Sources{Repo: repo, Templates: templates, TemplatesDir: "tmpl"})

			list := diag.From(err)
			var codes []diag.Code
			for _, d := range list {
				codes = append(codes, d.Code)
			}
			if !slices.Equal(codes, tt.wantCodes) {
				t.Fatalf("Plan() problems = %v, want %v\n%v", codes, tt.wantCodes, err)
			}
			for i, suffix := range tt.wantFiles {
				if !strings.HasSuffix(list[i].Pos.File, suffix) {
					t.Errorf("problem %d is in %q, want a file ending in %q", i, list[i].Pos.File, suffix)
				}
			}
			if tt.wantCodes == nil {
				if got := string(p.Changes[0].New); got != tt.want {
					t.Errorf("content = %q, want %q", got, tt.want)
				}
			}
		})
	}
}

func TestPlanEach(t *testing.T) {
	repo := fstest.MapFS{
		"stores/a/a.go": {Data: []byte("package a\n\nfunc NewStore() {}\n")},
		"stores/b/b.go": {Data: []byte("package b\n\nfunc NewStore() {}\n")},
	}
	templates := fstest.MapFS{
		"mock.go.tmpl":   {Data: []byte("package {{ .Item.Package }}\n\n// {{ .Item.Name }} has {{ len .Stores }} siblings.\n")},
		"other.go.tmpl":  {Data: []byte("package other\n")},
		"marker.go.tmpl": {Data: []byte("// Code generated by mockgen. DO NOT EDIT.\n\npackage {{ .Item.Package }}\n")},
	}
	const each = "{{ .Item.Dir }}/{{ .Item.Name | toSnakeCase }}_mock.gen.go"
	generated := func(template, pkg string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(header.Format(template) + "\n\npackage " + pkg + "\n")}
	}
	at := diag.Pos{File: "adder.yaml", Line: 9}

	tests := []struct {
		name     string
		path     string // the output's path; empty means each
		template string // the output's template; empty means mock.go.tmpl
		existing fstest.MapFS
		// extra is another output, written before the one with each.
		extra       *config.Output
		noItems     bool     // the scan matches nothing
		wantChanges []string // "status path"
		wantCodes   []diag.Code
	}{
		{
			name:        "one file per item",
			wantChanges: []string{"created stores/a/new_store_mock.gen.go", "created stores/b/new_store_mock.gen.go"},
		},
		{
			name: "removes files it generated before",
			existing: fstest.MapFS{
				"stores/old/new_store_mock.gen.go": generated("mock.go.tmpl", "old"),
				"stores/a/new_store_mock.gen.go":   generated("mock.go.tmpl", "a"),
			},
			wantChanges: []string{"updated stores/a/new_store_mock.gen.go", "created stores/b/new_store_mock.gen.go", "removed stores/old/new_store_mock.gen.go"},
		},
		{
			name: "leaves files it did not generate",
			existing: fstest.MapFS{
				"stores/x/handwritten_mock.gen.go": {Data: []byte("package x\n")},
				"stores/y/other_mock.gen.go":       generated("other.go.tmpl", "y"),
				"stores/z/new_store.go":            generated("mock.go.tmpl", "z"),
				"elsewhere/new_store_mock.gen.go":  generated("mock.go.tmpl", "elsewhere"),
				"stores/testdata/x_mock.gen.go":    generated("mock.go.tmpl", "x"),
				"stores/.hidden/x_mock.gen.go":     generated("mock.go.tmpl", "x"),
				"stores/nested/go.mod":             {Data: []byte("module example.com/nested\n")},
				"stores/nested/x_mock.gen.go":      generated("mock.go.tmpl", "x"),
				"stores/link_mock.gen.go":          {Data: []byte("a/new_store_mock.gen.go"), Mode: fs.ModeSymlink},
			},
			path:        "stores/{{ .Item.Dir }}_mock.gen.go",
			wantChanges: []string{"created stores/stores/a_mock.gen.go", "created stores/stores/b_mock.gen.go"},
		},
		{
			name:        "the search starts below the path's literal directory",
			path:        "gen/mocks/{{ .Item.Package }}.gen.go",
			existing:    fstest.MapFS{"gen/old.gen.go": generated("mock.go.tmpl", "old"), "gen/mocks/old.gen.go": generated("mock.go.tmpl", "old")},
			wantChanges: []string{"created gen/mocks/a.gen.go", "created gen/mocks/b.gen.go", "removed gen/mocks/old.gen.go"},
		},
		{
			name:      "never replaces a file adder did not generate",
			existing:  fstest.MapFS{"stores/a/new_store_mock.gen.go": {Data: []byte("package a\n\n// Handwritten.\n")}},
			wantCodes: []diag.Code{diag.FileExists},
		},
		{
			name:        "replaces a file adder generated from another template, as after a rename",
			existing:    fstest.MapFS{"stores/a/new_store_mock.gen.go": generated("old/mock.go.tmpl", "a")},
			wantChanges: []string{"updated stores/a/new_store_mock.gen.go", "created stores/b/new_store_mock.gen.go"},
		},
		{
			name:        "adds adder's header when the template writes another tool's",
			template:    "marker.go.tmpl",
			wantChanges: []string{"created stores/a/new_store_mock.gen.go", "created stores/b/new_store_mock.gen.go"},
		},
		{
			name:      "never replaces a file with another tool's header",
			template:  "marker.go.tmpl",
			existing:  fstest.MapFS{"stores/a/new_store_mock.gen.go": {Data: []byte("// Code generated by mockgen. DO NOT EDIT.\n\npackage a\n")}},
			wantCodes: []diag.Code{diag.FileExists},
		},
		{
			name:      "items sharing a path",
			path:      "stores/mock.gen.go",
			wantCodes: []diag.Code{diag.DuplicatePath},
		},
		{
			name:      "an item on another output's path",
			extra:     &config.Output{Template: "other.go.tmpl", Path: "stores/a/new_store_mock.gen.go", Pos: at},
			wantCodes: []diag.Code{diag.DuplicatePath},
		},
		{
			name:      "a path that is not a file name",
			path:      "{{ if false }}x{{ end }}",
			wantCodes: []diag.Code{diag.InvalidValue, diag.InvalidValue},
		},
		{
			name:      "a path outside the root",
			path:      "../{{ .Item.Package }}.go",
			wantCodes: []diag.Code{diag.UnsafePath, diag.UnsafePath},
		},
		{
			name:      "a path that does not render",
			path:      "{{ .Item.Nope }}.go",
			wantCodes: []diag.Code{diag.TemplateExec, diag.TemplateExec},
		},
		{
			name:      "a symbolic link on the way to an output",
			existing:  fstest.MapFS{"stores/a": {Data: []byte("b"), Mode: fs.ModeSymlink}, "stores/b/x.go": {}},
			wantCodes: []diag.Code{diag.UnsafePath},
		},
		{
			name:      "items under a symbolic link",
			path:      "links/{{ .Item.Package }}.gen.go",
			existing:  fstest.MapFS{"links": {Data: []byte("stores"), Mode: fs.ModeSymlink}},
			wantCodes: []diag.Code{diag.UnsafePath, diag.UnsafePath},
		},
		{
			name:      "a search starting at a symbolic link",
			path:      "links/{{ .Item.Package }}.gen.go",
			noItems:   true,
			existing:  fstest.MapFS{"links": {Data: []byte("stores"), Mode: fs.ModeSymlink}},
			wantCodes: []diag.Code{diag.UnsafePath},
		},
		{
			name:        "a scan without items removes every file the output generated",
			noItems:     true,
			existing:    fstest.MapFS{"stores/a/new_store_mock.gen.go": generated("mock.go.tmpl", "a")},
			wantChanges: []string{"removed stores/a/new_store_mock.gen.go"},
		},
		{
			name:     "a search below a missing directory finds nothing",
			path:     "missing/{{ .Item.Package }}.gen.go",
			noItems:  true,
			existing: fstest.MapFS{"other.go": {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := config.Output{Template: cmp.Or(tt.template, "mock.go.tmpl"), Path: cmp.Or(tt.path, each), Each: "Stores", Pos: at}
			name := regexp.MustCompile("^New")
			if tt.noItems {
				name = regexp.MustCompile("^Nothing$")
			}
			cfg := &config.Config{Version: 1, Derive: config.Derive{
				Scans:   map[string]config.Scan{"Stores": {Dirs: []string{"stores/*"}, Funcs: config.FuncMatch{Name: config.Pattern{Regexp: name}}}},
				Outputs: []config.Output{out},
			}}
			if tt.extra != nil {
				cfg.Derive.Outputs = []config.Output{*tt.extra, out}
			}
			src := Sources{Repo: repo, Templates: templates}
			if tt.existing != nil {
				src.Existing = tt.existing
			}

			p, err := Plan(t.Context(), cfg, src)
			var codes []diag.Code
			for _, d := range diag.From(err) {
				codes = append(codes, d.Code)
				if d.Pos != at && d.Pos.File == "" {
					t.Errorf("problem %q has no position", d.Message)
				}
			}
			if !slices.Equal(codes, tt.wantCodes) {
				t.Fatalf("Plan() problems = %v, want %v (error: %v)", codes, tt.wantCodes, err)
			}
			if err != nil {
				return
			}
			var got []string
			for _, c := range p.Changes {
				if c.Path != "stores/a/new_store_mock.gen.go" || tt.extra == nil {
					got = append(got, c.Status.String()+" "+c.Path)
				}
			}
			if !slices.Equal(got, tt.wantChanges) {
				t.Errorf("changes = %q, want %q", got, tt.wantChanges)
			}
			for _, c := range p.Changes {
				if c.Status == plan.Created {
					if source, ok := header.Parse(c.Path, c.New); !ok || source != out.Template {
						t.Errorf("%s has header %q, %v, want %s", c.Path, source, ok, out.Template)
					}
				}
			}
		})
	}
}

// TestPlanSkipsOwnOutputs checks that an output never feeds back into the scans that produce it, even when its
// template writes another tool's generated-code comment.
func TestPlanSkipsOwnOutputs(t *testing.T) {
	tests := []struct {
		name     string
		template string
	}{
		{name: "plain template", template: "package c\n\nfunc Run() {}\n// {{ len .Cmds }} commands\n"},
		{name: "template with another tool's marker", template: "// Code generated by mockgen. DO NOT EDIT.\n\npackage c\n\nfunc Run() {}\n// {{ len .Cmds }} commands\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := fstest.MapFS{"cmds/a/a.go": {Data: []byte("package a\n\nfunc Run() {}\n")}}
			cfg := &config.Config{Version: 1, Derive: config.Derive{
				Scans:   map[string]config.Scan{"Cmds": {Dirs: []string{"cmds/*"}}},
				Outputs: []config.Output{{Template: "c.go.tmpl", Path: "cmds/c/c.gen.go"}},
			}}
			src := Sources{Repo: repo, Templates: fstest.MapFS{"c.go.tmpl": {Data: []byte(tt.template)}}, Existing: repo}

			first, err := Plan(t.Context(), cfg, src)
			if err != nil {
				t.Fatal(err)
			}
			repo["cmds/c/c.gen.go"] = &fstest.MapFile{Data: first.Changes[0].New}
			second, err := Plan(t.Context(), cfg, src)
			if err != nil {
				t.Fatal(err)
			}
			if got := statuses(second); !slices.Equal(got, []plan.Status{plan.Unchanged}) {
				t.Errorf("second plan = %v, want the output unchanged, not scanned:\n%s", got, second.Changes[0].New)
			}
		})
	}
}

func TestExplain(t *testing.T) {
	templates := fstest.MapFS{
		"list.go.tmpl": {Data: []byte("package cmds\n\n// {{ len .Cmds }} commands, {{ .Vars }}\n")},
		"mock.go.tmpl": {Data: []byte("package {{ .Item.Package }}\n")},
	}
	repo := fstest.MapFS{
		"go.mod":               {Data: []byte("module example.com/m\n")},
		"cmds/a/a.go":          {Data: []byte("package a\n\nfunc Run() {}\n")},
		"cmds/b/b.go":          {Data: []byte("package b\n\nfunc Run() {}\n")},
		"handwritten":          {Data: []byte("text\n")},
		"cmds/list.go":         {Data: []byte(header.Format("list.go.tmpl") + "\n\npackage cmds\n\n// 2 commands, map[]\n")},
		"cmds/old.go":          {Data: []byte(header.Format("list.go.tmpl") + "\n\npackage cmds\n")},
		"gone.go":              {Data: []byte(header.Format("removed.go.tmpl") + "\n\npackage gone\n")},
		"cmds/a/mock.gen.go":   {Data: []byte(header.Format("mock.go.tmpl") + "\n\npackage a\n")},
		"cmds/z/mock.gen.go":   {Data: []byte(header.Format("mock.go.tmpl") + "\n\npackage z\n")},
		"testdata/mock.gen.go": {Data: []byte(header.Format("mock.go.tmpl") + "\n\npackage testdata\n")},
	}
	at := diag.Pos{File: "adder.yaml", Line: 7}
	cfg := func() *config.Config {
		return &config.Config{Version: 1, Derive: config.Derive{
			Scans: map[string]config.Scan{"Cmds": {Dirs: []string{"cmds/*"}}},
			Outputs: []config.Output{
				{Template: "list.go.tmpl", Path: "cmds/list.go", Pos: at},
				{Template: "mock.go.tmpl", Path: "{{ .Item.Dir }}/mock.gen.go", Each: "Cmds", Pos: at},
			},
		}}
	}
	status := func(s plan.Status) *plan.Status { return &s }

	tests := []struct {
		name          string
		file          string
		setup         func(cfg *config.Config)
		wantGenerated bool
		wantTemplate  string
		wantOutput    string // the output's path, empty when none writes the file
		wantScans     []string
		wantStatus    *plan.Status
		wantCode      diag.Code
		noTemplates   bool // the template files are missing
	}{
		{name: "a file adder did not generate", file: "handwritten"},
		{
			name: "an output that is up to date", file: "cmds/list.go",
			wantGenerated: true, wantTemplate: "list.go.tmpl", wantOutput: "cmds/list.go", wantScans: []string{"Cmds"}, wantStatus: status(plan.Unchanged),
		},
		{
			name: "an output that is out of date", file: "cmds/list.go",
			setup:         func(cfg *config.Config) { cfg.Vars = map[string]string{"A": "1"} },
			wantGenerated: true, wantTemplate: "list.go.tmpl", wantOutput: "cmds/list.go", wantScans: []string{"Cmds"}, wantStatus: status(plan.Updated),
		},
		{
			name: "a file per item", file: "cmds/a/mock.gen.go",
			wantGenerated: true, wantTemplate: "mock.go.tmpl", wantOutput: "{{ .Item.Dir }}/mock.gen.go", wantScans: []string{}, wantStatus: status(plan.Unchanged),
		},
		{
			name: "a file per item for an item that is gone", file: "cmds/z/mock.gen.go",
			wantGenerated: true, wantTemplate: "mock.go.tmpl", wantOutput: "{{ .Item.Dir }}/mock.gen.go", wantScans: []string{}, wantStatus: status(plan.Removed),
		},
		{name: "a template no output uses", file: "gone.go", wantGenerated: true, wantTemplate: "removed.go.tmpl"},
		{name: "a matching file where stale cleanup never looks", file: "testdata/mock.gen.go", wantGenerated: true, wantTemplate: "mock.go.tmpl"},
		{name: "an output's template at another path", file: "cmds/old.go", wantGenerated: true, wantTemplate: "list.go.tmpl"},
		{name: "a missing file", file: "missing.go", wantCode: diag.IO},
		{name: "a missing template", file: "cmds/list.go", noTemplates: true, wantCode: diag.IO},
		{
			name:     "partials that do not load",
			file:     "cmds/list.go",
			setup:    func(cfg *config.Config) { cfg.Partials = []config.Partial{{Pattern: "partials/*.tmpl", Pos: at}} },
			wantCode: diag.NoMatch,
		},
		{
			name: "a plan that fails", file: "cmds/list.go",
			setup: func(cfg *config.Config) {
				cfg.Derive.Scans["Extra"] = config.Scan{Dirs: []string{"missing/*"}, Pos: at}
			},
			wantCode: diag.NoMatch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cfg()
			if tt.setup != nil {
				tt.setup(c)
			}
			src := Sources{Repo: repo, Templates: templates, Existing: repo}
			if tt.noTemplates {
				src.Templates = fstest.MapFS{}
			}
			e, err := Explain(t.Context(), c, src, tt.file)
			if tt.wantCode != "" {
				if list := diag.From(err); len(list) == 0 || list[0].Code != tt.wantCode {
					t.Errorf("Explain() error = %v, want %s", err, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var output string
			if e.Output != nil {
				output = e.Output.Path
			}
			if e.Path != tt.file || e.Generated != tt.wantGenerated || e.Template != tt.wantTemplate || output != tt.wantOutput ||
				!slices.Equal(e.Scans, tt.wantScans) || (e.Status == nil) != (tt.wantStatus == nil) || (e.Status != nil && *e.Status != *tt.wantStatus) {
				t.Errorf("Explain() = %+v (status %v), want generated %v from %q by %q, scans %v, status %v",
					e, e.Status, tt.wantGenerated, tt.wantTemplate, tt.wantOutput, tt.wantScans, tt.wantStatus)
			}
		})
	}
}

func TestOpen(t *testing.T) {
	root, cfg := copyRepo(t, "basic")
	tests := []struct {
		name     string
		root     string
		dir      string // the configuration's directory
		wantFile string // the file named by the problem, empty for none
	}{
		{name: "the repository and its templates", root: root, dir: cfg.Dir},
		{name: "a missing root", root: filepath.Join(root, "missing"), dir: cfg.Dir, wantFile: filepath.Join(root, "missing")},
		{name: "a missing template directory", root: root, dir: filepath.Join(root, "missing"), wantFile: filepath.Join(root, "missing")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := *cfg
			c.Dir = tt.dir
			src, closeSources, err := Open(&c, tt.root, nil)
			if tt.wantFile != "" {
				if list := diag.From(err); len(list) != 1 || list[0].Code != diag.IO || list[0].Pos.File != tt.wantFile {
					t.Errorf("Open() error = %v, want an io problem for %s", err, tt.wantFile)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer closeSources()
			if _, err := Plan(t.Context(), &c, src); err != nil {
				t.Errorf("Plan() over the opened sources: %v", err)
			}
			if src.RepoDir != root || src.ExistingDir != root || src.TemplatesDir != cfg.Dir {
				t.Errorf("directories = %q, %q, %q", src.RepoDir, src.ExistingDir, src.TemplatesDir)
			}
		})
	}
}
