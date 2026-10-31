package pack

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"typhonhill.dev/adder/internal/diag"
)

func TestNew(t *testing.T) {
	shout := strings.ToUpper
	scanner := func(context.Context, ScanInput) ([]any, error) { return nil, nil }
	tests := []struct {
		name         string
		packs        []Pack
		wantFuncs    []string
		wantFS       []string
		wantScanners []string
		// wantDeprecated says what to use instead of each deprecated function; wantDeprecatedScanners, scanner.
		wantDeprecated         map[string]string
		wantDeprecatedScanners map[string]string
		wantErrs               []string
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
		{
			name:         "scanners from several packs",
			packs:        []Pack{{Name: "a", Scanners: map[string]Scanner{"routes": scanner}}, {Name: "b", Scanners: map[string]Scanner{"sql-tables": scanner}}},
			wantScanners: []string{"routes", "sql-tables"},
		},
		{name: "a scanner name that is not lowercase words", packs: []Pack{{Name: "a", Scanners: map[string]Scanner{"Routes": scanner}}}, wantErrs: []string{`pack "a": scanner name "Routes" is not lowercase words joined by hyphens`}},
		{name: "a nil scanner", packs: []Pack{{Name: "a", Scanners: map[string]Scanner{"routes": nil}}}, wantErrs: []string{`pack "a": scanner "routes" is nil`}},
		{name: "a scanner in two packs", packs: []Pack{{Name: "a", Scanners: map[string]Scanner{"routes": scanner}}, {Name: "b", Scanners: map[string]Scanner{"routes": scanner}}}, wantErrs: []string{`packs "a" and "b" both define scanner "routes"`}},
		{
			name:                   "a deprecated function and scanner",
			packs:                  []Pack{{Name: "a", Funcs: map[string]any{"shout": shout}, Scanners: map[string]Scanner{"env": scanner}, DeprecatedFuncs: map[string]string{"shout": "use upper"}, DeprecatedScanners: map[string]string{"env": "use dotenv"}}},
			wantFuncs:              []string{"shout"},
			wantScanners:           []string{"env"},
			wantDeprecated:         map[string]string{"shout": "use upper"},
			wantDeprecatedScanners: map[string]string{"env": "use dotenv"},
		},
		{
			name:                   "a function and a scanner of one name, only one deprecated",
			packs:                  []Pack{{Name: "a", Funcs: map[string]any{"env": shout}, Scanners: map[string]Scanner{"env": scanner}, DeprecatedScanners: map[string]string{"env": "use dotenv"}}},
			wantFuncs:              []string{"env"},
			wantScanners:           []string{"env"},
			wantDeprecatedScanners: map[string]string{"env": "use dotenv"},
		},
		{name: "deprecating a function the pack does not define", packs: []Pack{{Name: "a", DeprecatedFuncs: map[string]string{"shout": "use upper"}}}, wantErrs: []string{`pack "a": DeprecatedFuncs names "shout", which is not one of its functions`}},
		{name: "deprecating a function as a scanner", packs: []Pack{{Name: "a", Funcs: map[string]any{"env": shout}, DeprecatedScanners: map[string]string{"env": "use dotenv"}}}, wantErrs: []string{`pack "a": DeprecatedScanners names "env", which is not one of its scanners`}},
		{name: "deprecating without saying what to use instead", packs: []Pack{{Name: "a", Funcs: map[string]any{"shout": shout}, DeprecatedFuncs: map[string]string{"shout": ""}}}, wantErrs: []string{`pack "a": DeprecatedFuncs must say what to use instead of "shout"`}},
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
			if funcs := slices.Sorted(maps.Keys(r.Funcs())); !slices.Equal(funcs, tt.wantFuncs) {
				t.Errorf("Funcs() = %v, want %v", funcs, tt.wantFuncs)
			}
			for _, p := range tt.packs {
				if _, ok := r.FS(p.Name); ok != slices.Contains(tt.wantFS, p.Name) {
					t.Errorf("FS(%q) found = %v", p.Name, ok)
				}
			}
			for _, name := range append([]string{"missing"}, tt.wantScanners...) {
				if got, want := r.HasScanner(name), name != "missing"; got != want {
					t.Errorf("HasScanner(%q) = %v, want %v", name, got, want)
				}
				if got := r.ScannerDeprecation(name); got != tt.wantDeprecatedScanners[name] {
					t.Errorf("ScannerDeprecation(%q) = %q, want %q", name, got, tt.wantDeprecatedScanners[name])
				}
			}
			if !maps.Equal(r.DeprecatedFuncs(), tt.wantDeprecated) {
				t.Errorf("DeprecatedFuncs() = %v, want %v", r.DeprecatedFuncs(), tt.wantDeprecated)
			}
		})
	}
}

// TestNilRegistry checks the registry of a build without packs. It has a single case, so it is not a table.
func TestNilRegistry(t *testing.T) {
	var r *Registry
	if _, ok := r.FS("store"); ok {
		t.Error("FS() found a pack")
	}
	if r.HasScanner("routes") {
		t.Error("HasScanner() = true")
	}
	if r.ScannerDeprecation("env") != "" || r.DeprecatedFuncs() != nil {
		t.Error("a nil registry has deprecations")
	}
}
