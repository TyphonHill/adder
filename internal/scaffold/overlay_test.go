package scaffold

import (
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"
)

func TestOverlay(t *testing.T) {
	base := fstest.MapFS{
		"go.mod":           {Data: []byte("module m\n")},
		"drivers/s3/s3.go": {Data: []byte("package s3\n")},
	}
	tests := []struct {
		name  string
		files map[string][]byte
		dir   string
		want  []string // entries of dir
	}{
		{name: "no files", files: nil, dir: "drivers", want: []string{"s3"}},
		{
			name:  "a new directory",
			files: map[string][]byte{"drivers/gcs/gcs.go": []byte("package gcs\n")},
			dir:   "drivers",
			want:  []string{"gcs", "s3"},
		},
		{
			name:  "a new file in an existing directory",
			files: map[string][]byte{"drivers/s3/cache.go": []byte("package s3\n")},
			dir:   "drivers/s3",
			want:  []string{"cache.go", "s3.go"},
		},
		{
			name:  "a file replacing one in the base",
			files: map[string][]byte{"go.mod": []byte("module other\n")},
			dir:   ".",
			want:  []string{"drivers", "go.mod"},
		},
		{
			name:  "a directory only the overlay has",
			files: map[string][]byte{"new/deep/a.go": []byte("package a\n")},
			dir:   "new",
			want:  []string{"deep"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := overlay{base: base, files: tt.files}
			var expected []string
			for name := range tt.files {
				expected = append(expected, name)
			}
			if err := fstest.TestFS(o, append(expected, "drivers/s3/s3.go")...); err != nil {
				t.Fatal(err)
			}

			entries, err := fs.ReadDir(o, tt.dir)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, e := range entries {
				got = append(got, e.Name())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ReadDir(%q) = %v, want %v", tt.dir, got, tt.want)
			}
			for name, want := range tt.files {
				data, err := fs.ReadFile(o, name)
				if err != nil || string(data) != string(want) {
					t.Errorf("ReadFile(%q) = %q, %v, want %q", name, data, err, want)
				}
			}
		})
	}
}

func TestOverlayMissing(t *testing.T) {
	o := overlay{base: fstest.MapFS{}, files: map[string][]byte{"a/b.go": nil}}
	for _, name := range []string{"missing", "a/missing", "../a"} {
		if _, err := o.Open(name); err == nil {
			t.Errorf("Open(%q) succeeded", name)
		}
		if _, err := o.Stat(name); err == nil {
			t.Errorf("Stat(%q) succeeded", name)
		}
		if _, err := o.ReadDir(name); err == nil {
			t.Errorf("ReadDir(%q) succeeded", name)
		}
	}
}
