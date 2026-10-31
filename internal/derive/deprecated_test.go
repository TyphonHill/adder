package derive

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/pack"
)

func TestPlanWarnings(t *testing.T) {
	store := fstest.MapFS{
		"pack.yaml":    {Data: []byte("version: 1\nderive:\n  outputs:\n    - {template: list.go.tmpl, path: x/list.gen.go}\n")},
		"list.go.tmpl": {Data: []byte("package x\n\n// {{ shout \"x\" }}\n")},
	}
	packs, err := pack.New([]pack.Pack{
		{Name: "store", FS: store},
		{Name: "local", Funcs: map[string]any{"shout": strings.ToUpper, "upper": strings.ToUpper}, DeprecatedFuncs: map[string]string{"shout": "use upper"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := fstest.MapFS{"stores/a/a.go": {Data: []byte("package a\n\nfunc NewA() {}\n")}, "stores/b/b.go": {Data: []byte("package b\n\nfunc NewB() {}\n")}}
	templates := fstest.MapFS{
		"plain.go.tmpl": {Data: []byte("package x\n\n// {{ upper \"x\" }}\n")},
		"old.go.tmpl":   {Data: []byte("package x\n\n// {{ shout \"x\" }}\n")},
		"each.go.tmpl":  {Data: []byte("package {{ .Item.Package }}\n\n// {{ shout .Item.Name }}\n")},
	}
	scan := "  scans:\n    Ctors: {dirs: [\"stores/*\"], funcs: {name: ^New}}\n"
	tests := []struct {
		name   string
		config string
		want   []string // each warning, as pos: message
	}{
		{name: "nothing deprecated", config: "version: 1\nderive:\n" + scan + "  outputs:\n    - {template: plain.go.tmpl, path: x/x.gen.go}\n"},
		{
			name:   "in the configuration's own template",
			config: "version: 1\nderive:\n" + scan + "  outputs:\n    - {template: old.go.tmpl, path: x/x.gen.go}\n",
			want:   []string{`old.go.tmpl:3:7: function "shout" is deprecated: use upper`},
		},
		{
			name:   "once for an output with each",
			config: "version: 1\nderive:\n" + scan + "  outputs:\n    - {template: each.go.tmpl, each: Ctors, path: \"{{ .Item.Dir }}/x.gen.go\"}\n",
			want:   []string{`each.go.tmpl:3:7: function "shout" is deprecated: use upper`},
		},
		{
			name:   "in a pack's template",
			config: "version: 1\npacks:\n  lists: {pack: store}\n",
			want:   []string{`pack store: list.go.tmpl:3:7: function "shout" is deprecated: use upper`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Parse("adder.yaml", []byte(tt.config), packs)
			if err != nil {
				t.Fatal(err)
			}
			var w diag.Warnings
			ctx := diag.WithWarnings(t.Context(), &w)
			if _, err := Plan(ctx, cfg, Sources{Repo: repo, Templates: templates, Packs: packs}); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, d := range w.List() {
				got = append(got, d.Error())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("warnings = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTemplateDataVarsAndOptions(t *testing.T) {
	const tmpl = "package x\n\n// vars {{ len .Vars }}, options {{ len .Options }}\n"
	store := fstest.MapFS{
		"pack.yaml":    {Data: []byte("version: 1\noptions: {out: {}}\nderive:\n  outputs:\n    - {template: t.go.tmpl, path: \"{{ .Options.out }}/x.gen.go\"}\n")},
		"t.go.tmpl":    {Data: []byte(tmpl)},
		"own.go.tmpl":  {Data: []byte(tmpl)},
		"with.go.tmpl": {Data: []byte(tmpl)},
	}
	packs, err := pack.New([]pack.Pack{{Name: "store", FS: store}})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		config string
		want   string // the comment line of the one output
	}{
		{name: "the configuration's own template", config: "version: 1\nvars: {A: a}\nderive:\n  outputs:\n    - {template: own.go.tmpl, path: x/x.gen.go}\n", want: "// vars 1, options 0"},
		{name: "a pack's template under a use", config: "version: 1\nvars: {A: a}\npacks:\n  x: {pack: store, with: {out: x}}\n", want: "// vars 0, options 1"},
		{name: "a pack's template with the configuration's data", config: "version: 1\nvars: {A: a}\nderive:\n  outputs:\n    - {template: with.go.tmpl, pack: store, path: x/x.gen.go}\n", want: "// vars 1, options 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Parse("adder.yaml", []byte(tt.config), packs)
			if err != nil {
				t.Fatal(err)
			}
			templates := fstest.MapFS{"own.go.tmpl": {Data: []byte(tmpl)}}
			p, err := Plan(t.Context(), cfg, Sources{Repo: fstest.MapFS{}, Templates: templates, Packs: packs})
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Changes) != 1 || !strings.Contains(string(p.Changes[0].New), tt.want) {
				t.Errorf("changes = %+v, want one with %q", p.Changes, tt.want)
			}
		})
	}
}
