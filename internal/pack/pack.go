// Package pack registers the packs adder was built with.
package pack

import (
	"context"
	"io/fs"
	"maps"
	"slices"
	"text/template"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/render"
)

// Pack is a pack as the root package's adder.Pack describes it.
type Pack struct {
	Name     string
	FS       fs.FS
	Funcs    map[string]any
	Scanners map[string]Scanner
	// DeprecatedFuncs and DeprecatedScanners say what to use instead of the deprecated functions and scanners,
	// by name.
	DeprecatedFuncs, DeprecatedScanners map[string]string
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
	// deprecatedFuncs and deprecatedScanners say what to use instead of each deprecated function and scanner.
	deprecatedFuncs, deprecatedScanners map[string]string
}

// New checks packs and registers them, reporting every problem as an invalid-pack diagnostic.
func New(packs []Pack) (*Registry, error) {
	r := &Registry{
		funcs: template.FuncMap{}, fs: make(map[string]fs.FS), scanners: make(map[string]Scanner),
		deprecatedFuncs: make(map[string]string), deprecatedScanners: make(map[string]string),
	}
	seen := make(map[string]bool)
	funcFrom := make(map[string]string)
	scannerFrom := make(map[string]string)
	var problems diag.List
	invalid := func(format string, args ...any) {
		problems = append(problems, diag.Errorf(diag.InvalidPack, diag.Pos{}, format, args...))
	}
	for _, p := range packs {
		switch {
		case !config.IsName(p.Name):
			invalid("pack name %q is not lowercase words joined by hyphens", p.Name)
			continue
		case seen[p.Name]:
			invalid("two packs are named %q", p.Name)
			continue
		}
		seen[p.Name] = true
		if p.FS != nil {
			if info, err := fs.Stat(p.FS, config.PackFileName); err != nil || !info.Mode().IsRegular() {
				invalid("pack %q: its FS has no %s at its root", p.Name, config.PackFileName)
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
			case !config.IsName(s):
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
		deprecate := func(field, kind string, deprecated map[string]string, defined func(string) bool, into map[string]string) {
			for _, n := range slices.Sorted(maps.Keys(deprecated)) {
				switch instead := deprecated[n]; {
				case !defined(n):
					invalid("pack %q: %s names %q, which is not one of its %s", p.Name, field, n, kind)
				case instead == "":
					invalid("pack %q: %s must say what to use instead of %q", p.Name, field, n)
				default:
					into[n] = instead
				}
			}
		}
		deprecate("DeprecatedFuncs", "functions", p.DeprecatedFuncs, func(n string) bool { _, ok := p.Funcs[n]; return ok }, r.deprecatedFuncs)
		deprecate("DeprecatedScanners", "scanners", p.DeprecatedScanners, func(n string) bool { _, ok := p.Scanners[n]; return ok }, r.deprecatedScanners)
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

// DeprecatedFuncs says what to use instead of each deprecated function the packs define, by name.
func (r *Registry) DeprecatedFuncs() map[string]string {
	if r == nil {
		return nil
	}
	return r.deprecatedFuncs
}

// ScannerDeprecation says what to use instead of the scanner called name, or "" when it is not deprecated.
func (r *Registry) ScannerDeprecation(name string) string {
	if r == nil {
		return ""
	}
	return r.deprecatedScanners[name]
}
