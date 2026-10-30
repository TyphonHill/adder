package scaffold

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/derive"
	"github.com/TyphonHill/adder/internal/diag"
)

type testPacks map[string]fstest.MapFS

func (p testPacks) FS(name string) (fs.FS, bool) {
	fsys, ok := p[name]
	return fsys, ok
}

func (testPacks) Funcs() template.FuncMap { return nil }

func TestPlanPackGenerator(t *testing.T) {
	pack := func(file string) fstest.MapFS {
		return fstest.MapFS{
			"pack.yaml": {Data: []byte(`version: 1
options:
  dir: {}
  dirs: {type: list}
derive:
  scans:
    Impls: {dirs: {option: dirs}, funcs: {name: ^New$}}
  outputs:
    - {template: registry.go.tmpl, path: "{{ .Options.dir }}/registry.gen.go"}
generators:
  impl:
    inputs:
      name: {pattern: "[a-z]+", unique: Impls.Package}
    files:
      - {template: impl.go.tmpl, path: "` + file + `"}
    message: "{{ len .Impls }} in {{ .Options.dir }}"
`)},
			"registry.go.tmpl": {Data: []byte("package {{ .Options.dir }}\n\n// {{ range .Impls }}{{ .Package }} {{ end }}\n")},
			"impl.go.tmpl":     {Data: []byte("package {{ .Inputs.name }}\n\nfunc New() { {{ .Missing }} }\n")},
		}
	}
	repo := fstest.MapFS{"drivers/disk/disk.go": {Data: []byte("package disk\n\nfunc New() {}\n")}}
	tests := []struct {
		name     string
		file     string
		inputs   map[string]any
		wantMsg  string
		wantErrs []string
	}{
		{name: "a problem in the pack's template", file: "{{ .Options.dir }}/{{ .Inputs.name }}/x.go", inputs: map[string]any{"name": "gcs"}, wantErrs: []string{"pack store: impl.go.tmpl:3:16 template-exec"}},
		{name: "a file at a use's output", file: "{{ .Options.dir }}/registry.gen.go", inputs: map[string]any{"name": "gcs"}, wantErrs: []string{"pack store: pack.yaml:15:9 duplicate-path"}},
		{name: "unique against the use's scan", file: "x.go", inputs: map[string]any{"name": "disk"}, wantErrs: []string{"invalid-input"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Parse("adder.yaml", []byte("version: 1\npacks:\n  drivers: {pack: store, with: {dir: drivers, dirs: [\"drivers/*\"]}}\n"), testPacks{"store": pack(tt.file)})
			if err != nil {
				t.Fatal(err)
			}
			_, err = Plan(t.Context(), cfg, derive.Sources{Repo: repo, Templates: fstest.MapFS{}, Existing: repo}, "impl", tt.inputs)
			var errs []string
			for _, d := range diag.From(err) {
				errs = append(errs, strings.TrimSpace(d.Pos.String()+" "+string(d.Code)))
			}
			if !slices.Equal(errs, tt.wantErrs) {
				t.Errorf("Plan() problems = %q, want %q\n%v", errs, tt.wantErrs, err)
			}
		})
	}
}

func TestPlanPackTemplateFile(t *testing.T) {
	pack := fstest.MapFS{
		"pack.yaml": {Data: []byte("version: 1\n")},
		"doc.tmpl":  {Data: []byte("// {{ .Inputs.name }} for {{ .Vars.Team }}\npackage {{ .Inputs.name }}\n")},
		"bad.tmpl":  {Data: []byte("{{ .Nope }}")},
	}
	tests := []struct {
		name     string
		template string
		want     string
		wantErrs []string
	}{
		{name: "the configuration's own data", template: "doc.tmpl", want: "// gcs for storage\npackage gcs\n"},
		{name: "a problem in the pack's template", template: "bad.tmpl", wantErrs: []string{"pack store: bad.tmpl:1:3 template-exec"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Parse("adder.yaml", []byte("version: 1\nvars: {Team: storage}\ngenerators:\n  doc:\n    inputs: {name: {}}\n    files: [{template: "+tt.template+", pack: store, path: \"{{ .Inputs.name }}/doc.go\"}]\n"), testPacks{"store": pack})
			if err != nil {
				t.Fatal(err)
			}
			res, err := Plan(t.Context(), cfg, derive.Sources{Repo: fstest.MapFS{}, Templates: fstest.MapFS{}}, "doc", map[string]any{"name": "gcs"})
			var errs []string
			for _, d := range diag.From(err) {
				errs = append(errs, d.Pos.String()+" "+string(d.Code))
			}
			if !slices.Equal(errs, tt.wantErrs) {
				t.Fatalf("Plan() problems = %q, want %q\n%v", errs, tt.wantErrs, err)
			}
			if err == nil && (len(res.Plan.Changes) != 1 || string(res.Plan.Changes[0].New) != tt.want) {
				t.Errorf("changes = %+v", res.Plan.Changes)
			}
		})
	}
}
