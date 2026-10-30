package plan

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/TyphonHill/adder/internal/diag"
)

type failingFS struct{}

func (failingFS) Open(string) (fs.File, error) { return nil, fs.ErrPermission }

func TestAdd(t *testing.T) {
	existing := fstest.MapFS{
		"same.go":    {Data: []byte("same")},
		"changed.go": {Data: []byte("old")},
	}
	tests := []struct {
		name       string
		existing   fs.FS
		path       string
		content    string
		wantStatus Status
		wantOld    []byte
		wantErr    error
	}{
		{name: "same content", existing: existing, path: "same.go", content: "same", wantStatus: Unchanged, wantOld: []byte("same")},
		{name: "different content", existing: existing, path: "changed.go", content: "new", wantStatus: Updated, wantOld: []byte("old")},
		{name: "new file", existing: existing, path: "new/file.go", content: "new", wantStatus: Created},
		{name: "no existing files", existing: nil, path: "a.go", content: "a", wantStatus: Created},
		{name: "read error", existing: failingFS{}, path: "a.go", wantErr: fs.ErrPermission},
		{name: "a directory where the file goes", existing: fstest.MapFS{"dir/a.go": {}}, path: "dir", wantErr: fs.ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Plan
			err := p.Add(tt.existing, tt.path, []byte(tt.content))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Add() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Add() error = %v", err)
			}
			got := p.Changes[0]
			if got.Path != tt.path || got.Status != tt.wantStatus || !slices.Equal(got.Old, tt.wantOld) || string(got.New) != tt.content {
				t.Errorf("change = %+v, want %s with old content %q", got, tt.wantStatus, tt.wantOld)
			}
		})
	}
}

func TestPending(t *testing.T) {
	tests := []struct {
		name    string
		changes []Change
		want    []string
	}{
		{name: "empty", changes: nil, want: nil},
		{name: "only unchanged", changes: []Change{{Path: "a", Status: Unchanged}}, want: nil},
		{name: "skipped files are not written", changes: []Change{{Path: "a", Status: Skipped}}, want: nil},
		{name: "removed files", changes: []Change{{Path: "a", Status: Removed}}, want: []string{"a"}},
		{
			name:    "mixed, in order",
			changes: []Change{{Path: "a", Status: Updated}, {Path: "b", Status: Unchanged}, {Path: "c", Status: Created}},
			want:    []string{"a", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Plan{Changes: tt.changes}
			var got []string
			for _, c := range p.Pending() {
				got = append(got, c.Path)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Pending() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApply(t *testing.T) {
	tests := []struct {
		name      string
		initial   map[string]string // files on disk before applying
		links     map[string]string // symbolic links on disk before applying, by name, to their targets
		readOnly  string            // a directory made read-only before applying, which Windows does not enforce
		changes   []Change
		canceled  bool
		want      map[string]string // files on disk after applying
		wantGone  []string          // files that must not exist after applying
		wantErr   bool
		wantErrIs error
	}{
		{
			name:    "creates nested files",
			changes: []Change{{Path: "nested/dir/file.go", Status: Created, New: []byte("new")}},
			want:    map[string]string{"nested/dir/file.go": "new"},
		},
		{
			name:    "updates files",
			initial: map[string]string{"a.go": "old"},
			changes: []Change{{Path: "a.go", Status: Updated, Old: []byte("old"), New: []byte("new")}},
			want:    map[string]string{"a.go": "new"},
		},
		{
			name:    "skips unchanged files",
			initial: map[string]string{"a.go": "on disk"},
			changes: []Change{{Path: "a.go", Status: Unchanged, New: []byte("not written")}},
			want:    map[string]string{"a.go": "on disk"},
		},
		{
			name:    "leaves skipped files alone",
			initial: map[string]string{"a.go": "on disk"},
			changes: []Change{{Path: "a.go", Status: Skipped, New: []byte("not written")}},
			want:    map[string]string{"a.go": "on disk"},
		},
		{
			name:     "removes files",
			initial:  map[string]string{"a.go": "old", "b.go": "kept"},
			changes:  []Change{{Path: "a.go", Status: Removed, Old: []byte("old")}},
			want:     map[string]string{"b.go": "kept"},
			wantGone: []string{"a.go"},
		},
		{
			name:    "a directory in the way of a new file",
			initial: map[string]string{"a": "file"},
			changes: []Change{{Path: "a/b.go", Status: Created, New: []byte("x")}},
			wantErr: true,
		},
		{
			name:     "a directory that cannot be created",
			initial:  map[string]string{"ro/a.go": "x"},
			readOnly: "ro",
			changes:  []Change{{Path: "ro/sub/b.go", Status: Created, New: []byte("x")}},
			wantErr:  true,
		},
		{
			name:    "a removed path that is a directory with files",
			initial: map[string]string{"d/x.go": "kept"},
			changes: []Change{{Path: "d", Status: Removed}},
			want:    map[string]string{"d/x.go": "kept"},
			wantErr: true,
		},
		{
			name:    "a removed file that is already gone",
			changes: []Change{{Path: "a.go", Status: Removed, Old: []byte("old")}},
		},
		{
			name:    "never writes through a linked directory",
			initial: map[string]string{"real/a.go": "owned"},
			links:   map[string]string{"link": "real"},
			changes: []Change{{Path: "link/a.go", Status: Updated, Old: []byte("owned"), New: []byte("new")}},
			want:    map[string]string{"real/a.go": "owned"},
			wantErr: true,
		},
		{
			name:    "never removes a linked file",
			initial: map[string]string{"real.go": "owned"},
			links:   map[string]string{"a.go": "real.go"},
			changes: []Change{{Path: "a.go", Status: Removed, Old: []byte("owned")}},
			want:    map[string]string{"real.go": "owned", "a.go": "owned"},
			wantErr: true,
		},
		{
			name:      "never overwrites a file created since planning",
			initial:   map[string]string{"a.go": "on disk"},
			changes:   []Change{{Path: "a.go", Status: Created, New: []byte("new")}},
			want:      map[string]string{"a.go": "on disk"},
			wantErr:   true,
			wantErrIs: fs.ErrExist,
		},
		{
			name:    "stays inside the root",
			changes: []Change{{Path: "../escaped.go", Status: Created, New: []byte("x")}},
			wantErr: true,
		},
		{
			name:      "canceled",
			changes:   []Change{{Path: "a.go", Status: Created, New: []byte("x")}},
			canceled:  true,
			wantErr:   true,
			wantErrIs: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.initial {
				file := filepath.Join(dir, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.readOnly != "" {
				if runtime.GOOS == "windows" {
					t.Skip("Windows does not enforce read-only directories")
				}
				ro := filepath.Join(dir, tt.readOnly)
				if err := os.Chmod(ro, 0o555); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(ro, 0o755) }) // so TempDir can remove it
			}
			for name, target := range tt.links {
				if err := os.Symlink(target, filepath.Join(dir, filepath.FromSlash(name))); err != nil {
					t.Skipf("cannot create symbolic links here: %v", err)
				}
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { root.Close() })

			ctx := t.Context()
			if tt.canceled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			p := Plan{Changes: tt.changes}
			err = p.Apply(ctx, root)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Apply() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("Apply() error = %v, want %v", err, tt.wantErrIs)
			}
			for name, want := range tt.want {
				got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
				if err != nil || string(got) != want {
					t.Errorf("%s = %q, %v, want %q", name, got, err, want)
				}
			}
			for _, name := range tt.wantGone {
				if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(name))); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("%s still exists (%v)", name, err)
				}
			}
		})
	}
}

func TestCheckLinks(t *testing.T) {
	fsys := fstest.MapFS{
		"dir/file.go": {Data: []byte("x")},
		"dirlink":     {Data: []byte("dir"), Mode: fs.ModeSymlink},
		"filelink.go": {Data: []byte("dir/file.go"), Mode: fs.ModeSymlink},
	}
	tests := []struct {
		name     string
		fsys     fs.FS // nil means the links above
		path     string
		wantCode diag.Code // empty means no problem
	}{
		{name: "a file", path: "dir/file.go"},
		{name: "a file that does not exist yet", path: "new/dir/file.go"},
		{name: "a linked file", path: "filelink.go", wantCode: diag.UnsafePath},
		{name: "a file in a linked directory", path: "dirlink/file.go", wantCode: diag.UnsafePath},
		{name: "a linked directory", path: "dirlink", wantCode: diag.UnsafePath},
		{name: "an unreadable file system", fsys: failingFS{}, path: "a/b.go", wantCode: diag.IO},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckLinks(cmp.Or[fs.FS](tt.fsys, fsys), tt.path)
			var got diag.Code
			if list := diag.From(err); len(list) > 0 {
				got = list[0].Code
				if list[0].Pos.File != tt.path {
					t.Errorf("problem is at %v, want %s", list[0].Pos, tt.path)
				}
			}
			if got != tt.wantCode {
				t.Errorf("CheckLinks(%q) = %v, want code %q", tt.path, err, tt.wantCode)
			}
		})
	}
}

func TestChangeDiff(t *testing.T) {
	tests := []struct {
		name   string
		change Change
		want   string
	}{
		{
			name:   "created",
			change: Change{Path: "a/b.go", Status: Created, New: []byte("package b\n")},
			want:   "diff /dev/null b/a/b.go\n--- /dev/null\n+++ b/a/b.go\n@@ -0,0 +1,1 @@\n+package b\n",
		},
		{
			name:   "updated",
			change: Change{Path: "x.txt", Status: Updated, Old: []byte("one\ntwo\n"), New: []byte("one\n2\n")},
			want:   "diff a/x.txt b/x.txt\n--- a/x.txt\n+++ b/x.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+2\n",
		},
		{name: "unchanged", change: Change{Path: "x.txt", Status: Unchanged, Old: []byte("x\n"), New: []byte("x\n")}},
		{name: "skipped", change: Change{Path: "x.txt", Status: Skipped, New: []byte("x\n")}},
		{
			name:   "removed",
			change: Change{Path: "x.txt", Status: Removed, Old: []byte("x\n")},
			want:   "diff a/x.txt /dev/null\n--- a/x.txt\n+++ /dev/null\n@@ -1,1 +0,0 @@\n-x\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(tt.change.Diff()); got != tt.want {
				t.Errorf("Diff() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     Status
		wantString string
		wantText   string
		wantErr    bool
	}{
		{name: "unchanged", status: Unchanged, wantString: "unchanged", wantText: "unchanged"},
		{name: "created", status: Created, wantString: "created", wantText: "created"},
		{name: "updated", status: Updated, wantString: "updated", wantText: "updated"},
		{name: "skipped", status: Skipped, wantString: "skipped", wantText: "skipped"},
		{name: "removed", status: Removed, wantString: "removed", wantText: "removed"},
		{name: "unknown", status: Status(9), wantString: "Status(9)", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.String(); got != tt.wantString {
				t.Errorf("String() = %q, want %q", got, tt.wantString)
			}
			text, err := tt.status.MarshalText()
			if (err != nil) != tt.wantErr {
				t.Fatalf("MarshalText() error = %v, wantErr %v", err, tt.wantErr)
			}
			if string(text) != tt.wantText {
				t.Errorf("MarshalText() = %q, want %q", text, tt.wantText)
			}
		})
	}
}

func TestPlanJSON(t *testing.T) {
	tests := []struct {
		name string
		plan Plan
		want string
	}{
		{
			name: "content is left out",
			plan: Plan{Changes: []Change{{Path: "a.go", Status: Created, Old: []byte("old"), New: []byte("new")}}},
			want: `{"changes":[{"path":"a.go","status":"created"}]}`,
		},
		{
			name: "several changes in order",
			plan: Plan{Changes: []Change{{Path: "b.go", Status: Unchanged}, {Path: "a.go", Status: Updated}}},
			want: `{"changes":[{"path":"b.go","status":"unchanged"},{"path":"a.go","status":"updated"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.plan)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("json = %s, want %s", got, tt.want)
			}
		})
	}
}
