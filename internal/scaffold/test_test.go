package scaffold

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/derive"
	"github.com/TyphonHill/adder/internal/diag"
)

func TestGeneratorTest(t *testing.T) {
	const want = "testdata/generators/driver/plain"
	example := "// Package example is a driver that stores objects. Licensed under Apache-2.0.\npackage example\n\nimport \"example.com/storage/driver\"\n\n// New returns a example driver.\nfunc New(config map[string]string) (driver.Driver, error) { return nil, nil }\n"
	readme := "# Drivers\n\nEach directory holds one driver.\n"
	tests := []struct {
		name        string
		golden      map[string]string // files under want, by path in the repository
		inputs      map[string]any
		setup       func(t *testing.T, root string)
		wantChanges []string // "status path", with paths under want
		wantCode    diag.Code
	}{
		{
			name:   "files match",
			golden: map[string]string{"drivers/example/example.go": example, "drivers/README.md": readme},
		},
		{
			name:        "no golden files yet",
			wantChanges: []string{"created " + want + "/drivers/README.md", "created " + want + "/drivers/example/example.go"},
		},
		{
			name:        "a golden file that differs",
			golden:      map[string]string{"drivers/example/example.go": "old\n", "drivers/README.md": readme},
			wantChanges: []string{"updated " + want + "/drivers/example/example.go"},
		},
		{
			name:        "a golden file the generator no longer creates",
			golden:      map[string]string{"drivers/example/example.go": example, "drivers/README.md": readme, "drivers/example/cache.go": "x"},
			wantChanges: []string{"removed " + want + "/drivers/example/cache.go"},
		},
		{
			name: "files the generator skips are not compared",
			setup: func(t *testing.T, root string) {
				writeFile(t, filepath.Join(root, "drivers", "README.md"), "mine\n")
			},
			golden: map[string]string{"drivers/example/example.go": example},
		},
		{
			name:     "inputs the generator rejects",
			inputs:   map[string]any{"name": "s3"},
			wantCode: diag.InvalidInput,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, cfg := copyRepo(t)
			if tt.setup != nil {
				tt.setup(t, root)
			}
			for name, content := range tt.golden {
				writeFile(t, filepath.Join(root, filepath.FromSlash(want), filepath.FromSlash(name)), content)
			}
			inputs := tt.inputs
			if inputs == nil {
				inputs = map[string]any{"name": "example"}
			}
			test := config.GeneratorTest{Name: "plain", Inputs: inputs, Want: want, Pos: diag.Pos{File: "adder.yaml", Line: 30}}
			src, closeSources, err := derive.Open(cfg, root, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer closeSources()

			res, err := Test(t.Context(), cfg, src, "driver", test)
			if tt.wantCode != "" {
				list := diag.From(err)
				if len(list) != 1 || list[0].Code != tt.wantCode || list[0].Pos != test.Pos {
					t.Errorf("Test() error = %v, want a %s problem at the test", err, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, c := range res.Changes {
				got = append(got, c.Status.String()+" "+c.Path)
			}
			if !slices.Equal(got, tt.wantChanges) || res.Passed() != (len(tt.wantChanges) == 0) {
				t.Errorf("changes = %q, passed %v, want %q", got, res.Passed(), tt.wantChanges)
			}
		})
	}
}

func TestGoldenFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a", "b.go"), "b")
	writeFile(t, filepath.Join(dir, "c.txt"), "c")
	if err := os.Symlink("c.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("cannot create symbolic links here: %v", err)
	}
	fsys := os.DirFS(dir)
	tests := []struct {
		name string
		dir  string
		want []string
	}{
		{name: "a directory", dir: "a", want: []string{"b.go"}},
		{name: "the root, without links", dir: ".", want: []string{"a/b.go", "c.txt"}},
		{name: "a missing directory", dir: "missing", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := goldenFiles(fsys, tt.dir)
			if err != nil {
				t.Fatal(err)
			}
			got := slices.Sorted(maps.Keys(files))
			if !slices.Equal(got, tt.want) {
				t.Errorf("goldenFiles(%q) = %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
}
