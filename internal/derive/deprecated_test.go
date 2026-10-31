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
		want   []string // each warning, as position and code
	}{
		{name: "nothing deprecated", config: "version: 1\nderive:\n" + scan + "  outputs:\n    - {template: plain.go.tmpl, path: x/x.gen.go}\n"},
		{
			name:   "in the configuration's own template",
			config: "version: 1\nderive:\n" + scan + "  outputs:\n    - {template: old.go.tmpl, path: x/x.gen.go}\n",
			want:   []string{"old.go.tmpl:3:7 deprecated"},
		},
		{
			name:   "once for an output with each",
			config: "version: 1\nderive:\n" + scan + "  outputs:\n    - {template: each.go.tmpl, each: Ctors, path: \"{{ .Item.Dir }}/x.gen.go\"}\n",
			want:   []string{"each.go.tmpl:3:7 deprecated"},
		},
		{
			name:   "in the path of an output with each",
			config: "version: 1\nderive:\n" + scan + "  outputs:\n    - {template: plain.go.tmpl, each: Ctors, path: \"x/{{ shout .Item.Name }}.gen.go\"}\n",
			want:   []string{"adder.yaml:6:7 deprecated"},
		},
		{
			name:   "in a pack's template",
			config: "version: 1\npacks:\n  lists: {pack: store}\n",
			want:   []string{"pack store: list.go.tmpl:3:7 deprecated"},
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
				got = append(got, d.Pos.String()+" "+string(d.Code))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("warnings = %q, want %q", got, tt.want)
			}
		})
	}
}
