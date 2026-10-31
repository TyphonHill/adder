package adder

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"runtime/debug"
	"slices"
	"testing"
	"testing/fstest"

	"typhonhill.dev/adder/internal/pack"
)

func TestVersionIn(t *testing.T) {
	tests := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{name: "no build information", want: "(devel)"},
		{name: "adder's own binary", info: &debug.BuildInfo{Main: debug.Module{Path: module, Version: "v0.5.0"}}, want: "v0.5.0"},
		{
			name: "a repository's own build",
			info: &debug.BuildInfo{Main: debug.Module{Path: "example.com/app"}, Deps: []*debug.Module{{Path: "example.com/other", Version: "v1.0.0"}, {Path: module, Version: "v0.5.1"}}},
			want: "v0.5.1",
		},
		{
			name: "a replaced module",
			info: &debug.BuildInfo{Main: debug.Module{Path: "example.com/app"}, Deps: []*debug.Module{{Path: module, Version: "v0.5.1", Replace: &debug.Module{Path: "../adder"}}}},
			want: "(devel)",
		},
		{
			name: "a module replaced by another version",
			info: &debug.BuildInfo{Main: debug.Module{Path: "example.com/app"}, Deps: []*debug.Module{{Path: module, Version: "v0.5.1", Replace: &debug.Module{Path: "example.com/fork", Version: "v0.5.2"}}}},
			want: "v0.5.2",
		},
		{name: "adder is not a dependency", info: &debug.BuildInfo{Main: debug.Module{Path: "example.com/app"}}, want: "(devel)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionIn(tt.info); got != tt.want {
				t.Errorf("versionIn() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInternal(t *testing.T) {
	// Every field of Pack must reach the internal packages.
	if n := reflect.TypeFor[Pack]().NumField(); n != reflect.TypeFor[pack.Pack]().NumField() {
		t.Errorf("Pack has %d fields, but pack.Pack has %d", n, reflect.TypeFor[pack.Pack]().NumField())
	}
	same := func(s string) string { return s }
	tests := []struct {
		name  string
		packs []Pack
	}{
		{name: "none"},
		{name: "every field", packs: []Pack{{
			Name:               "local",
			Funcs:              map[string]any{"same": same},
			Scanners:           map[string]Scanner{"routes": func(context.Context, *ScanInput) ([]any, error) { return nil, nil }},
			DeprecatedFuncs:    map[string]string{"same": "use other"},
			DeprecatedScanners: map[string]string{"routes": "use paths"},
		}, {Name: "store", FS: fstest.MapFS{}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := internal(tt.packs)
			if len(got) != len(tt.packs) {
				t.Fatalf("internal() = %d packs, want %d", len(got), len(tt.packs))
			}
			for i, p := range tt.packs {
				if got[i].Name != p.Name || len(got[i].Funcs) != len(p.Funcs) || (got[i].FS == nil) != (p.FS == nil) || len(got[i].Scanners) != len(p.Scanners) ||
					!maps.Equal(got[i].DeprecatedFuncs, p.DeprecatedFuncs) || !maps.Equal(got[i].DeprecatedScanners, p.DeprecatedScanners) {
					t.Errorf("internal()[%d] = %+v, want %+v", i, got[i], p)
				}
			}
		})
	}
}

func TestProblems(t *testing.T) {
	other := errors.New("no routes file")
	tests := []struct {
		name string
		err  error
		want []string // "position code message"
	}{
		{name: "a problem", err: Problem{File: "api/routes.go", Line: 3, Column: 2, Message: "bad route"}, want: []string{"api/routes.go:3:2 scanner bad route"}},
		{name: "a pointer to a problem", err: &Problem{File: "api/routes.go", Message: "bad route"}, want: []string{"api/routes.go scanner bad route"}},
		{name: "joined problems", err: errors.Join(Problem{File: "a.go", Line: 1, Message: "one"}, Problem{File: "b.go", Line: 2, Message: "two"}), want: []string{"a.go:1 scanner one", "b.go:2 scanner two"}},
		{name: "a wrapped problem", err: fmt.Errorf("routes: %w", Problem{File: "a.go", Message: "bad"}), want: []string{"a.go scanner bad"}},
		{name: "another error", err: other, want: []string{" error no routes file"}},
		{name: "a problem joined with another error", err: errors.Join(Problem{File: "a.go", Message: "bad"}, other), want: []string{"a.go scanner bad", " error no routes file"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, d := range problems(tt.err) {
				got = append(got, d.Pos.String()+" "+string(d.Code)+" "+d.Message)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("problems() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProblemError(t *testing.T) {
	tests := []struct {
		name string
		p    Problem
		want string
	}{
		{name: "with a position", p: Problem{File: "a.go", Line: 3, Column: 2, Message: "bad"}, want: "a.go:3:2: bad"},
		{name: "without one", p: Problem{Message: "bad"}, want: "bad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScanInputDecode(t *testing.T) {
	type settings struct{ Prefix string }
	tests := []struct {
		name string
		in   *ScanInput
		want settings
	}{
		{name: "built by hand, without settings", in: &ScanInput{}, want: settings{Prefix: "unchanged"}},
		{name: "with settings", in: &ScanInput{decode: func(v any) error { v.(*settings).Prefix = "/v1"; return nil }}, want: settings{Prefix: "/v1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := settings{Prefix: "unchanged"}
			if err := tt.in.Decode(&got); err != nil || got != tt.want {
				t.Errorf("Decode() = %+v, %v, want %+v", got, err, tt.want)
			}
		})
	}
}
