package scaffold

import (
	"errors"
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
	tests := []struct {
		name string
		path string
	}{
		{name: "a missing file", path: "missing"},
		{name: "a missing file beside a new one", path: "a/missing"},
		{name: "a path outside", path: "../a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := o.Open(tt.path); err == nil {
				t.Errorf("Open(%q) succeeded", tt.path)
			}
			if _, err := o.Stat(tt.path); err == nil {
				t.Errorf("Stat(%q) succeeded", tt.path)
			}
			if _, err := o.ReadDir(tt.path); err == nil {
				t.Errorf("ReadDir(%q) succeeded", tt.path)
			}
		})
	}
}

func TestOverlayLinks(t *testing.T) {
	base := fstest.MapFS{
		"drivers/s3/s3.go": {Data: []byte("package s3\n")},
		"link":             {Data: []byte("drivers"), Mode: fs.ModeSymlink},
	}
	o := overlay{base: base, files: map[string][]byte{"new/a.go": []byte("package a\n")}}
	if err := fstest.TestFS(o, "new/a.go", "drivers/s3/s3.go", "link"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		path       string
		wantMode   fs.FileMode
		wantTarget string
		wantErr    error // from ReadLink
	}{
		{name: "a link in the base", path: "link", wantMode: fs.ModeSymlink, wantTarget: "drivers"},
		{name: "a new file", path: "new/a.go", wantMode: 0, wantErr: fs.ErrInvalid},
		{name: "a new directory", path: "new", wantMode: fs.ModeDir, wantErr: fs.ErrInvalid},
		{name: "a file in the base", path: "drivers/s3/s3.go", wantMode: 0, wantErr: fs.ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := o.Lstat(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Type(); got != tt.wantMode {
				t.Errorf("Lstat(%q) type = %v, want %v", tt.path, got, tt.wantMode)
			}
			target, err := o.ReadLink(tt.path)
			if !errors.Is(err, tt.wantErr) || target != tt.wantTarget {
				t.Errorf("ReadLink(%q) = %q, %v, want %q, %v", tt.path, target, err, tt.wantTarget, tt.wantErr)
			}
		})
	}
}
