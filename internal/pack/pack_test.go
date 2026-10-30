package pack

import (
	"context"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/TyphonHill/adder/internal/diag"
)

func TestNew(t *testing.T) {
	shout := strings.ToUpper
	tests := []struct {
		name      string
		packs     []Pack
		wantFuncs []string
		wantFS    []string
		wantErrs  []string
	}{
		{name: "no packs"},
		{
			name:      "functions from several packs",
			packs:     []Pack{{Name: "local", Funcs: map[string]any{"shout": shout}}, {Name: "store-kit", Funcs: map[string]any{"quiet": strings.ToLower}}},
			wantFuncs: []string{"quiet", "shout"},
		},
		{name: "a pack without functions", packs: []Pack{{Name: "store", FS: fstest.MapFS{"pack.yaml": {Data: []byte("version: 1\n")}}}}, wantFS: []string{"store"}},
		{name: "an FS without pack.yaml", packs: []Pack{{Name: "store", FS: fstest.MapFS{"templates/x.tmpl": {}}}}, wantErrs: []string{`pack "store": its FS has no pack.yaml at its root`}},
		{name: "pack.yaml that is a directory", packs: []Pack{{Name: "store", FS: fstest.MapFS{"pack.yaml/x": {}}}}, wantErrs: []string{`pack "store": its FS has no pack.yaml at its root`}},
		{name: "a name that is not lowercase words", packs: []Pack{{Name: "Store"}}, wantErrs: []string{`pack name "Store" is not lowercase words joined by hyphens`}},
		{name: "an empty name", packs: []Pack{{}}, wantErrs: []string{`pack name "" is not lowercase words joined by hyphens`}},
		{name: "two packs with one name", packs: []Pack{{Name: "store"}, {Name: "store"}}, wantErrs: []string{`two packs are named "store"`}},
		{
			name:     "a function in two packs",
			packs:    []Pack{{Name: "a", Funcs: map[string]any{"shout": shout}}, {Name: "b", Funcs: map[string]any{"shout": shout}}},
			wantErrs: []string{`packs "a" and "b" both define function "shout"`},
		},
		{
			name:  "every problem with functions is reported",
			packs: []Pack{{Name: "local", Funcs: map[string]any{"join": shout, "range": shout, "bad name": shout, "two": func() (int, int) { return 0, 0 }}}},
			wantErrs: []string{
				`pack "local": function name "bad name" is not an identifier`,
				`pack "local": function "join" is built in`,
				`pack "local": function "range" is built in`,
				`pack "local": function "two" must return one value, or a value and an error`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New(tt.packs)
			var got []string
			for _, d := range diag.From(err) {
				if d.Code != diag.InvalidPack {
					t.Errorf("code = %s, want %s", d.Code, diag.InvalidPack)
				}
				got = append(got, d.Message)
			}
			if !slices.Equal(got, tt.wantErrs) {
				t.Fatalf("New() problems = %q, want %q", got, tt.wantErrs)
			}
			if err != nil {
				return
			}
			var funcs []string
			for name := range r.Funcs() {
				funcs = append(funcs, name)
			}
			slices.Sort(funcs)
			if !slices.Equal(funcs, tt.wantFuncs) {
				t.Errorf("Funcs() = %v, want %v", funcs, tt.wantFuncs)
			}
			for _, p := range tt.packs {
				if _, ok := r.FS(p.Name); ok != slices.Contains(tt.wantFS, p.Name) {
					t.Errorf("FS(%q) found = %v", p.Name, ok)
				}
			}
			if _, ok := (*Registry)(nil).FS("store"); ok {
				t.Error("a nil registry has a pack")
			}
		})
	}
}

func TestScanners(t *testing.T) {
	scanner := func(context.Context, ScanInput) ([]any, error) { return nil, nil }
	tests := []struct {
		name     string
		packs    []Pack
		want     []string
		wantErrs []string
	}{
		{name: "scanners from several packs", packs: []Pack{{Name: "a", Scanners: map[string]Scanner{"routes": scanner}}, {Name: "b", Scanners: map[string]Scanner{"sql-tables": scanner}}}, want: []string{"routes", "sql-tables"}},
		{name: "a name that is not lowercase words", packs: []Pack{{Name: "a", Scanners: map[string]Scanner{"Routes": scanner}}}, wantErrs: []string{`pack "a": scanner name "Routes" is not lowercase words joined by hyphens`}},
		{name: "a nil scanner", packs: []Pack{{Name: "a", Scanners: map[string]Scanner{"routes": nil}}}, wantErrs: []string{`pack "a": scanner "routes" is nil`}},
		{name: "a scanner in two packs", packs: []Pack{{Name: "a", Scanners: map[string]Scanner{"routes": scanner}}, {Name: "b", Scanners: map[string]Scanner{"routes": scanner}}}, wantErrs: []string{`packs "a" and "b" both define scanner "routes"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New(tt.packs)
			var got []string
			for _, d := range diag.From(err) {
				got = append(got, d.Message)
			}
			if !slices.Equal(got, tt.wantErrs) {
				t.Fatalf("New() problems = %q, want %q", got, tt.wantErrs)
			}
			for _, name := range tt.want {
				if !r.HasScanner(name) {
					t.Errorf("HasScanner(%q) = false", name)
				}
			}
			if r != nil && r.HasScanner("missing") || (*Registry)(nil).HasScanner("routes") {
				t.Error("HasScanner found a scanner no pack defines")
			}
		})
	}
}
