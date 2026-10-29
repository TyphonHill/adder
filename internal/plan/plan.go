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

	"github.com/TyphonHill/adder/internal/diag"
)

// Status is what applying a change does to its file.
type Status int

// Statuses of a change.
const (
	Unchanged Status = iota
	Created
	Updated
)

var statusNames = map[Status]string{Unchanged: "unchanged", Created: "created", Updated: "updated"}

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
	New    []byte `json:"-"`
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

// Pending returns the changes that would modify a file.
func (p *Plan) Pending() []Change {
	var pending []Change
	for _, c := range p.Changes {
		if c.Status != Unchanged {
			pending = append(pending, c)
		}
	}
	return pending
}

// Apply writes every pending change inside root.
func (p *Plan) Apply(ctx context.Context, root *os.Root) error {
	for _, c := range p.Pending() {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := filepath.FromSlash(c.Path)
		if dir := path.Dir(c.Path); dir != "." {
			if err := root.MkdirAll(filepath.FromSlash(dir), 0o755); err != nil {
				return diag.Wrap(diag.IO, diag.Pos{File: c.Path}, err)
			}
		}
		if err := root.WriteFile(name, c.New, 0o644); err != nil {
			return diag.Wrap(diag.IO, diag.Pos{File: c.Path}, err)
		}
	}
	return nil
}
