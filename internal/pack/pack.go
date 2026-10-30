// Package pack registers the packs adder was built with: derive outputs, templates, and template functions that
// repositories share.
package pack

import (
	"context"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"text/template"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/render"
)

// FileName is the configuration file at the root of a pack's file system.
const FileName = config.PackFileName

// Pack is a pack as the root package's adder.Pack describes it.
type Pack struct {
	Name     string
	FS       fs.FS
	Funcs    map[string]any
	Scanners map[string]Scanner
}

// Scanner returns the items of a scan that uses it, as the root package's adder.Scanner does.
type Scanner func(ctx context.Context, in ScanInput) ([]any, error)

// ScanInput is what a scanner reads.
type ScanInput struct {
	// FS is the repository, with the files adder new is about to write.
	FS fs.FS
	// Dirs are the scan's directories, expanded, slash-separated, and in order.
	Dirs []string
	// Decode decodes the scan's with settings into v, rejecting unknown fields.
	Decode func(v any) error
}

// Registry holds the packs adder was built with. A nil Registry has none.
type Registry struct {
	funcs    template.FuncMap
	fs       map[string]fs.FS
	scanners map[string]Scanner
}

// name is the form of a pack name, the same as a generator's.
var name = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// New checks packs and registers them, reporting every problem as an invalid-pack diagnostic.
func New(packs []Pack) (*Registry, error) {
	r := &Registry{funcs: template.FuncMap{}, fs: make(map[string]fs.FS), scanners: make(map[string]Scanner)}
	seen := make(map[string]bool)
	funcFrom := make(map[string]string)
	scannerFrom := make(map[string]string)
	var problems diag.List
	invalid := func(format string, args ...any) {
		problems = append(problems, diag.Errorf(diag.InvalidPack, diag.Pos{}, format, args...))
	}
	for _, p := range packs {
		switch {
		case !name.MatchString(p.Name):
			invalid("pack name %q is not lowercase words joined by hyphens", p.Name)
			continue
		case seen[p.Name]:
			invalid("two packs are named %q", p.Name)
			continue
		}
		seen[p.Name] = true
		if p.FS != nil {
			if info, err := fs.Stat(p.FS, FileName); err != nil || !info.Mode().IsRegular() {
				invalid("pack %q: its FS has no %s at its root", p.Name, FileName)
			} else {
				r.fs[p.Name] = p.FS
			}
		}
		for _, fn := range slices.Sorted(maps.Keys(p.Funcs)) {
			if err := render.CheckFunc(fn, p.Funcs[fn]); err != nil {
				invalid("pack %q: %v", p.Name, err)
				continue
			}
			if first, dup := funcFrom[fn]; dup {
				invalid("packs %q and %q both define function %q", first, p.Name, fn)
				continue
			}
			funcFrom[fn] = p.Name
			r.funcs[fn] = p.Funcs[fn]
		}
		for _, s := range slices.Sorted(maps.Keys(p.Scanners)) {
			first, dup := scannerFrom[s]
			switch {
			case !name.MatchString(s):
				invalid("pack %q: scanner name %q is not lowercase words joined by hyphens", p.Name, s)
			case p.Scanners[s] == nil:
				invalid("pack %q: scanner %q is nil", p.Name, s)
			case dup:
				invalid("packs %q and %q both define scanner %q", first, p.Name, s)
			default:
				scannerFrom[s] = p.Name
				r.scanners[s] = p.Scanners[s]
			}
		}
	}
	if err := problems.Err(); err != nil {
		return nil, err
	}
	return r, nil
}

// FS returns the file system of the pack called name, holding its pack.yaml and templates, if adder was built
// with it and it has one.
func (r *Registry) FS(name string) (fs.FS, bool) {
	if r == nil {
		return nil, false
	}
	fsys, ok := r.fs[name]
	return fsys, ok
}

// Scanner returns the scanner called name, if a pack defines it.
func (r *Registry) Scanner(name string) (Scanner, bool) {
	if r == nil {
		return nil, false
	}
	s, ok := r.scanners[name]
	return s, ok
}

// HasScanner reports whether a pack defines the scanner called name.
func (r *Registry) HasScanner(name string) bool {
	_, ok := r.Scanner(name)
	return ok
}

// Funcs returns the template functions the packs add to the built-in ones.
func (r *Registry) Funcs() template.FuncMap {
	if r == nil {
		return nil
	}
	return r.funcs
}
