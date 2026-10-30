package scaffold

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/derive"
	"github.com/TyphonHill/adder/internal/diag"
)

var update = flag.Bool("update", false, "rewrite golden files")

// copyRepo copies the fixture into a temporary directory, derives its outputs as a repository would have
// them committed, and loads its configuration.
func copyRepo(t *testing.T) (string, *config.Config) {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("testdata", "drivers", "repo"))); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := derive.Run(t.Context(), cfg, derive.Options{Root: root}); err != nil {
		t.Fatal(err)
	}
	return root, cfg
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name string
		// setup changes the copied fixture and its configuration; it may be nil.
		setup     func(t *testing.T, root string, cfg *config.Config)
		generator string // defaults to driver
		inputs    map[string]any
		dryRun    bool
		// wantChanges lists each change as "status path".
		wantChanges []string
		wantMessage string
		wantCodes   []diag.Code
		wantHint    string   // of the first problem, when set
		wantFiles   []string // diagnostic files, matched by suffix
		wantGolden  []string // files compared with testdata/drivers/golden after running
		// wantContent holds files whose content is known after running, by slash-separated path.
		wantContent map[string]string
	}{
		{
			name:        "creates files and updates derived outputs",
			inputs:      map[string]any{"name": "gcs"},
			wantChanges: []string{"created drivers/gcs/gcs.go", "created drivers/README.md", "updated drivers/drivers.gen.go"},
			wantMessage: "Next, implement New in drivers/gcs/gcs.go.\n",
			wantGolden:  []string{"drivers/gcs/gcs.go", "drivers/drivers.gen.go"},
		},
		{
			name:        "when includes a file",
			inputs:      map[string]any{"name": "gcs", "cache": true},
			wantChanges: []string{"created drivers/gcs/gcs.go", "created drivers/gcs/cache.go", "created drivers/README.md", "updated drivers/drivers.gen.go"},
			wantMessage: "Next, implement New in drivers/gcs/gcs.go.\n",
		},
		{
			name: "unless excludes a file",
			setup: func(_ *testing.T, _ string, cfg *config.Config) {
				gen := cfg.Generators["driver"]
				gen.Files[1].When, gen.Files[1].Unless = "", "cache"
			},
			inputs:      map[string]any{"name": "gcs", "cache": true},
			wantChanges: []string{"created drivers/gcs/gcs.go", "created drivers/README.md", "updated drivers/drivers.gen.go"},
			wantMessage: "Next, implement New in drivers/gcs/gcs.go.\n",
		},
		{
			name: "skips existing files marked skip",
			setup: func(t *testing.T, root string, _ *config.Config) {
				writeFile(t, filepath.Join(root, "drivers", "README.md"), "mine\n")
			},
			inputs:      map[string]any{"name": "gcs"},
			wantChanges: []string{"created drivers/gcs/gcs.go", "skipped drivers/README.md", "updated drivers/drivers.gen.go"},
			wantMessage: "Next, implement New in drivers/gcs/gcs.go.\n",
			wantContent: map[string]string{"drivers/README.md": "mine\n"},
		},
		{
			name: "without derive",
			setup: func(_ *testing.T, _ string, cfg *config.Config) {
				gen := cfg.Generators["driver"]
				gen.Derive = new(false)
				cfg.Generators["driver"] = gen
			},
			inputs:      map[string]any{"name": "gcs"},
			wantChanges: []string{"created drivers/gcs/gcs.go", "created drivers/README.md"},
			wantMessage: "Next, implement New in drivers/gcs/gcs.go.\n",
		},
		{
			name:        "dry run writes nothing",
			inputs:      map[string]any{"name": "gcs"},
			dryRun:      true,
			wantChanges: []string{"created drivers/gcs/gcs.go", "created drivers/README.md", "updated drivers/drivers.gen.go"},
			wantMessage: "Next, implement New in drivers/gcs/gcs.go.\n",
		},
		{
			name: "a file that already exists",
			setup: func(t *testing.T, root string, _ *config.Config) {
				writeFile(t, filepath.Join(root, "drivers", "gcs", "gcs.go"), "package gcs\n")
			},
			inputs:    map[string]any{"name": "gcs"},
			wantCodes: []diag.Code{diag.FileExists},
			wantFiles: []string{filepath.Join("drivers", "gcs", "gcs.go")},
		},
		{
			name:      "unknown generator",
			generator: "drivr",
			wantCodes: []diag.Code{diag.UnknownGenerator},
			wantHint:  "generators: driver",
		},
		{
			name:      "no generators",
			setup:     func(_ *testing.T, _ string, cfg *config.Config) { cfg.Generators = nil },
			wantCodes: []diag.Code{diag.UnknownGenerator},
			wantHint:  "no generators are defined; add them under generators in adder.yaml",
		},
		{
			name: "a generator without inputs",
			setup: func(_ *testing.T, _ string, cfg *config.Config) {
				gen := cfg.Generators["driver"]
				gen.Inputs = nil
				cfg.Generators["driver"] = gen
			},
			inputs:    map[string]any{"name": "gcs"},
			wantCodes: []diag.Code{diag.InvalidInput},
			wantHint:  "this generator takes no inputs",
		},
		{
			name:      "missing input",
			inputs:    map[string]any{},
			wantCodes: []diag.Code{diag.MissingInput},
			wantHint:  "Driver name, used as the package name",
		},
		{
			name:      "every input problem at once",
			inputs:    map[string]any{"nam": "gcs", "summary": 3, "cache": "yes"},
			wantCodes: []diag.Code{diag.InvalidInput, diag.InvalidInput, diag.MissingInput, diag.InvalidInput},
			wantHint:  "inputs: cache, name, summary",
		},
		{
			name:      "pattern matches the whole value",
			inputs:    map[string]any{"name": "gcs-2"},
			wantCodes: []diag.Code{diag.InvalidInput},
		},
		{
			name:      "unique rejects existing values",
			inputs:    map[string]any{"name": "s3"},
			wantCodes: []diag.Code{diag.InvalidInput},
		},
		{
			name: "values",
			setup: func(_ *testing.T, _ string, cfg *config.Config) {
				cfg.Generators["driver"].Inputs["summary"] = config.Input{Type: config.InputString, Values: []string{"fast", "slow"}}
			},
			inputs:    map[string]any{"name": "gcs", "summary": "medium"},
			wantCodes: []diag.Code{diag.InvalidInput},
		},
		{
			name: "valuesFrom accepts values a scan has",
			setup: func(_ *testing.T, _ string, cfg *config.Config) {
				cfg.Generators["driver"].Inputs["summary"] = config.Input{Type: config.InputString, ValuesFrom: "Drivers.Package"}
			},
			inputs:      map[string]any{"name": "gcs", "summary": "local"},
			wantChanges: []string{"created drivers/gcs/gcs.go", "created drivers/README.md", "updated drivers/drivers.gen.go"},
			wantMessage: "Next, implement New in drivers/gcs/gcs.go.\n",
		},
		{
			name: "valuesFrom rejects others",
			setup: func(_ *testing.T, _ string, cfg *config.Config) {
				cfg.Generators["driver"].Inputs["summary"] = config.Input{Type: config.InputString, ValuesFrom: "Drivers.Package"}
			},
			inputs:    map[string]any{"name": "gcs", "summary": "gcs"},
			wantCodes: []diag.Code{diag.InvalidInput},
		},
		{
			name: "valuesFrom with an unknown field",
			setup: func(_ *testing.T, _ string, cfg *config.Config) {
				cfg.Generators["driver"].Inputs["summary"] = config.Input{Type: config.InputString, ValuesFrom: "Drivers.Pkg", Pos: diag.Pos{File: "adder.yaml", Line: 9}}
			},
			inputs:    map[string]any{"name": "gcs", "summary": "local"},
			wantCodes: []diag.Code{diag.InvalidValue},
			wantHint:  "string fields: Dir, Doc, File, ImportPath, Kind, Name, Package, Receiver, Signature",
			wantFiles: []string{"adder.yaml"},
		},
		{
			name: "path problems",
			setup: func(_ *testing.T, _ string, cfg *config.Config) {
				gen := cfg.Generators["driver"]
				at := diag.Pos{File: "adder.yaml", Line: 9}
				gen.Files = []config.File{
					{Template: "templates/new/cache.go.tmpl", Path: "../{{ .Inputs.name }}.go", Pos: at},
					{Template: "templates/new/cache.go.tmpl", Path: "drivers/drivers.gen.go", Pos: at},
					{Template: "templates/new/cache.go.tmpl", Path: "{{ .Inputs.nam }}", Pos: at},
					{Template: "templates/new/cache.go.tmpl", Path: "drivers/{{ .Inputs.summary }}/", Pos: at},
					{Template: "templates/new/cache.go.tmpl", Path: "a.go", Pos: at},
					{Template: "templates/new/cache.go.tmpl", Path: "./a.go", Pos: at},
					{Template: "templates/new/missing.tmpl", Path: "b.go", Pos: at},
				}
				gen.Message = "{{ .Inputs.nope }}"
				cfg.Generators["driver"] = gen
			},
			inputs: map[string]any{"name": "gcs"},
			wantCodes: []diag.Code{
				diag.UnsafePath, diag.DuplicatePath, diag.TemplateExec, diag.InvalidValue, diag.DuplicatePath, diag.IO, diag.TemplateExec,
			},
			wantFiles: []string{"adder.yaml", "adder.yaml", "adder.yaml", "adder.yaml", "adder.yaml", filepath.Join("new", "missing.tmpl"), "adder.yaml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, cfg := copyRepo(t)
			if tt.setup != nil {
				tt.setup(t, root, cfg)
			}
			generator := tt.generator
			if generator == "" {
				generator = "driver"
			}

			res, err := Run(t.Context(), cfg, generator, tt.inputs, Options{Root: root, DryRun: tt.dryRun})

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
			if err != nil {
				if _, statErr := os.Stat(filepath.Join(root, "drivers", "README.md")); statErr == nil {
					t.Error("files were written despite problems")
				}
				return
			}

			var changes []string
			for _, c := range res.Plan.Changes {
				if c.Status.String() != "unchanged" {
					changes = append(changes, c.Status.String()+" "+c.Path)
				}
			}
			if !slices.Equal(changes, tt.wantChanges) {
				t.Errorf("changes = %q, want %q", changes, tt.wantChanges)
			}
			if res.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", res.Message, tt.wantMessage)
			}
			for _, c := range res.Plan.Pending() {
				_, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(c.Path)))
				if written := statErr == nil; written == tt.dryRun && c.Status.String() == "created" {
					t.Errorf("%s written = %v with dry run %v", c.Path, written, tt.dryRun)
				}
			}
			for file, want := range tt.wantContent {
				if got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file))); string(got) != want {
					t.Errorf("%s = %q, %v, want %q", file, got, err, want)
				}
			}

			for _, file := range tt.wantGolden {
				got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
				if err != nil {
					t.Fatal(err)
				}
				golden := filepath.Join("testdata", "drivers", "golden", filepath.Base(file))
				if *update {
					if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(golden, got, 0o644); err != nil {
						t.Fatal(err)
					}
				}
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != string(want) {
					t.Errorf("%s differs from %s (run go test -update to accept):\ngot:\n%s\nwant:\n%s", file, golden, got, want)
				}
			}
		})
	}
}
