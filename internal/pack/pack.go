// Package pack registers the packs adder was built with: derive outputs, templates, and template functions that
// repositories share.
package pack

import (
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"text/template"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/diag"
	"github.com/TyphonHill/adder/internal/render"
)

// FileName is the configuration file at the root of a pack's file system.
const FileName = config.PackFileName

// Pack is a pack as the root package's adder.Pack describes it.
type Pack struct {
	Name  string
	FS    fs.FS
	Funcs map[string]any
}

// Registry holds the packs adder was built with. A nil Registry has none.
type Registry struct {
	funcs template.FuncMap
	fs    map[string]fs.FS
}

// name is the form of a pack name, the same as a generator's.
var name = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// New checks packs and registers them, reporting every problem as an invalid-pack diagnostic.
func New(packs []Pack) (*Registry, error) {
	r := &Registry{funcs: template.FuncMap{}, fs: make(map[string]fs.FS)}
	seen := make(map[string]bool)
	funcFrom := make(map[string]string)
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

// Funcs returns the template functions the packs add to the built-in ones.
func (r *Registry) Funcs() template.FuncMap {
	if r == nil {
		return nil
	}
	return r.funcs
}
