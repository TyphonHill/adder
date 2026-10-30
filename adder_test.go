package adder

import (
	"reflect"
	"runtime/debug"
	"testing"
	"testing/fstest"

	"github.com/TyphonHill/adder/internal/pack"
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
	same := func(s string) string { return s }
	tests := []struct {
		name  string
		packs []Pack
	}{
		{name: "none"},
		{name: "every field", packs: []Pack{{Name: "local", Funcs: map[string]any{"same": same}}, {Name: "store", FS: fstest.MapFS{}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := internal(tt.packs)
			if len(got) != len(tt.packs) {
				t.Fatalf("internal() = %d packs, want %d", len(got), len(tt.packs))
			}
			for i, p := range tt.packs {
				if got[i].Name != p.Name || len(got[i].Funcs) != len(p.Funcs) || (got[i].FS == nil) != (p.FS == nil) {
					t.Errorf("internal()[%d] = %+v, want %+v", i, got[i], p)
				}
			}
			// Every field of Pack must reach the internal packages.
			if n := reflect.TypeFor[Pack]().NumField(); n != reflect.TypeFor[pack.Pack]().NumField() {
				t.Errorf("Pack has %d fields, but pack.Pack has %d", n, reflect.TypeFor[pack.Pack]().NumField())
			}
		})
	}
}
