package scaffold

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/derive"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/pack"
)

func TestPlanWarnings(t *testing.T) {
	packs, err := pack.New([]pack.Pack{{Name: "local", Funcs: map[string]any{"shout": strings.ToUpper, "upper": strings.ToUpper}, DeprecatedFuncs: map[string]string{"shout": "use upper"}}})
	if err != nil {
		t.Fatal(err)
	}
	templates := fstest.MapFS{
		"plain.go.tmpl": {Data: []byte("package {{ .Inputs.name }}\n\n// {{ upper .Inputs.name }}\n")},
		"old.go.tmpl":   {Data: []byte("package {{ .Inputs.name }}\n\n// {{ shout .Inputs.name }}\n")},
	}
	generator := func(template string) string {
		return "version: 1\ngenerators:\n  driver:\n    inputs: {name: {}}\n    files:\n      - {template: " + template + ", path: \"drivers/{{ .Inputs.name }}/x.go\"}\n"
	}
	tests := []struct {
		name   string
		config string
		want   []string // each warning, as pos: message
	}{
		{name: "nothing deprecated", config: generator("plain.go.tmpl")},
		{name: "in a file's template", config: generator("old.go.tmpl"), want: []string{`old.go.tmpl:3:7: function "shout" is deprecated: use upper`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Parse("adder.yaml", []byte(tt.config), packs)
			if err != nil {
				t.Fatal(err)
			}
			var w diag.Warnings
			ctx := diag.WithWarnings(t.Context(), &w)
			src := derive.Sources{Repo: fstest.MapFS{}, Templates: templates, Packs: packs}
			if _, err := Plan(ctx, cfg, src, "driver", map[string]any{"name": "gcs"}); err != nil {
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
