package pack

import (
	"slices"
	"strings"
	"testing"

	"github.com/TyphonHill/adder/internal/diag"
)

func TestNew(t *testing.T) {
	shout := strings.ToUpper
	tests := []struct {
		name      string
		packs     []Pack
		wantFuncs []string
		wantErrs  []string
	}{
		{name: "no packs"},
		{
			name:      "functions from several packs",
			packs:     []Pack{{Name: "local", Funcs: map[string]any{"shout": shout}}, {Name: "store-kit", Funcs: map[string]any{"quiet": strings.ToLower}}},
			wantFuncs: []string{"quiet", "shout"},
		},
		{name: "a pack without functions", packs: []Pack{{Name: "store"}}},
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
		})
	}
}
