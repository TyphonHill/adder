// Package plan describes file changes before they are made, so they can be previewed, checked, or applied.
package plan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/rogpeppe/go-internal/diff"

	"typhonhill.dev/adder/internal/diag"
)

// Status is what applying a change does to its file.
type Status int

// Statuses of a change.
const (
	Unchanged Status = iota
	Created
	Updated
	// Skipped is a new file that already exists and is left alone.
	Skipped
	// Removed is a file adder generated that its output no longer produces.
	Removed
)

var statusNames = map[Status]string{Unchanged: "unchanged", Created: "created", Updated: "updated", Skipped: "skipped", Removed: "removed"}

func (s Status) String() string {
	if name, ok := statusNames[s]; ok {
		return name
	}
	return fmt.Sprintf("Status(%d)", int(s))
}

// MarshalText encodes the status by name, for JSON output.
func (s Status) MarshalText() ([]byte, error) {
	if name, ok := statusNames[s]; ok {
		return []byte(name), nil
	}
	return nil, fmt.Errorf("unknown status %d", int(s))
}

// Change is the intended content of one file.
type Change struct {
	Path   string `json:"path"` // slash-separated, relative to the output root
	Status Status `json:"status"`
	Old    []byte `json:"-"` // current content, nil when the file does not exist
	New    []byte `json:"-"` // nil when the file is removed
}

// Diff returns the change as a unified diff that git apply and patch -p1 accept, or nil when it changes nothing.
func (c Change) Diff() []byte {
	switch c.Status {
	case Created:
		return diff.Diff("/dev/null", nil, "b/"+c.Path, c.New)
	case Updated:
		return diff.Diff("a/"+c.Path, c.Old, "b/"+c.Path, c.New)
	case Removed:
		return diff.Diff("a/"+c.Path, c.Old, "/dev/null", nil)
	}
	return nil
}

// Plan is an ordered set of changes.
type Plan struct {
	Changes []Change `json:"changes"`
}

// Add records new as the content of name, comparing it with the file currently in existing.
// A nil existing means no files exist yet.
func (p *Plan) Add(existing fs.FS, name string, content []byte) error {
	change := Change{Path: name, Status: Created, New: content}
	if existing != nil {
		if err := CheckLinks(existing, name); err != nil {
			return err
		}
		old, err := fs.ReadFile(existing, name)
		switch {
		case err == nil && bytes.Equal(old, content):
			change.Status, change.Old = Unchanged, old
		case err == nil:
			change.Status, change.Old = Updated, old
		case !errors.Is(err, fs.ErrNotExist):
			return diag.Wrap(diag.IO, diag.Pos{File: name}, err)
		}
	}
	p.Changes = append(p.Changes, change)
	return nil
}

// Pending returns the changes that would write or remove a file.
func (p *Plan) Pending() []Change {
	var pending []Change
	for _, c := range p.Changes {
		if c.Status == Created || c.Status == Updated || c.Status == Removed {
			pending = append(pending, c)
		}
	}
	return pending
}

// Apply makes every pending change inside root. It never writes or removes through a symbolic link.
func (p *Plan) Apply(ctx context.Context, root *os.Root) error {
	for _, c := range p.Pending() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := CheckLinks(root.FS(), c.Path); err != nil {
			return err
		}
		name := filepath.FromSlash(c.Path)
		if c.Status == Removed {
			if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return diag.Wrap(diag.IO, diag.Pos{File: c.Path}, err)
			}
			continue
		}
		if dir := path.Dir(c.Path); dir != "." {
			if err := root.MkdirAll(filepath.FromSlash(dir), 0o755); err != nil {
				return diag.Wrap(diag.IO, diag.Pos{File: c.Path}, err)
			}
		}
		if err := write(root, name, c); err != nil {
			return diag.Wrap(diag.IO, diag.Pos{File: c.Path}, err)
		}
	}
	return nil
}

// write writes a change's content. A created file must still not exist, so a file made since planning is never
// overwritten.
func write(root *os.Root, name string, c Change) error {
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if c.Status == Created {
		flag = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	f, err := root.OpenFile(name, flag, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(c.New)
	return errors.Join(err, f.Close())
}

// CheckLinks returns an unsafe-path problem when name, or a directory on the way to it, is a symbolic link in
// fsys. Writing through a link would change a file somewhere else, which may be one that people own.
func CheckLinks(fsys fs.FS, name string) error {
	for p := name; p != "." && p != "/"; p = path.Dir(p) {
		info, err := fs.Lstat(fsys, p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return diag.Wrap(diag.IO, diag.Pos{File: name}, err)
		case info.Mode()&fs.ModeSymlink != 0:
			return &diag.Diagnostic{
				Code:    diag.UnsafePath,
				Pos:     diag.Pos{File: name},
				Message: fmt.Sprintf("%s is a symbolic link", p),
				Hint:    "adder never writes or removes files through a symbolic link; replace the link with a directory or file",
			}
		}
	}
	return nil
}
