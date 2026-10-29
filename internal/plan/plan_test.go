package plan

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
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
		changes   []Change
		canceled  bool
		want      map[string]string // files on disk after applying
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
				if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
					t.Fatal(err)
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
