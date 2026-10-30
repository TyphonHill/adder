package scan

import (
	"io/fs"
	"path"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/diag"
)

// listing reads each directory of a file system once per scan, since expanding patterns, finding boundaries,
// scanning, and finding modules all list the same directories.
type listing struct {
	fsys    fs.FS
	entries map[string][]fs.DirEntry
	modules map[string]string // import paths of nested modules' directories, by directory
}

func newListing(fsys fs.FS) *listing {
	return &listing{fsys: fsys, entries: make(map[string][]fs.DirEntry), modules: make(map[string]string)}
}

func (l *listing) read(dir string) ([]fs.DirEntry, error) {
	if entries, ok := l.entries[dir]; ok {
		return entries, nil
	}
	entries, err := fs.ReadDir(l.fsys, dir)
	if err != nil {
		return nil, err
	}
	l.entries[dir] = entries
	return entries, nil
}

// has reports whether dir holds a regular file called name.
func (l *listing) has(dir, name string) bool {
	entries, err := l.read(dir)
	return err == nil && slices.ContainsFunc(entries, func(e fs.DirEntry) bool { return e.Name() == name && e.Type().IsRegular() })
}

func (l *listing) boundary(dir string) bool {
	return l.has(dir, config.FileName) || l.has(dir, "go.mod")
}

// importPath returns the import path of the package in dir: from the nearest go.mod between dir and the root, or
// from module, the root's module path, when there is none.
func (l *listing) importPath(module, dir string) string {
	nested, at := l.nearestModule(dir)
	if nested == "" {
		return importPath(module, dir)
	}
	rel := "."
	if at != dir {
		rel = strings.TrimPrefix(dir, at+"/")
	}
	return importPath(nested, rel)
}

// nearestModule returns the module path of the nearest go.mod between dir and the root, not counting the root's,
// and the directory holding it.
func (l *listing) nearestModule(dir string) (module, at string) {
	for d := dir; d != "."; d = path.Dir(d) {
		if mod, ok := l.modules[d]; ok {
			if mod != "" {
				return mod, d
			}
			continue
		}
		mod := ""
		if l.has(d, "go.mod") {
			if data, err := fs.ReadFile(l.fsys, path.Join(d, "go.mod")); err == nil {
				mod = modfile.ModulePath(data)
			}
		}
		l.modules[d] = mod
		if mod != "" {
			return mod, d
		}
	}
	return "", ""
}

// Dirs returns the directories the scan's dirs patterns match in fsys, without those it excludes, in order. A
// scan without dirs has none.
func Dirs(fsys fs.FS, s config.Scan) ([]string, error) {
	if len(s.Dirs) == 0 {
		return []string{}, nil
	}
	return expandDirs(newListing(fsys), s)
}

// expandDirs resolves the scan's directory patterns to unique directories, leaving out excluded ones.
func expandDirs(l *listing, s config.Scan) ([]string, error) {
	var dirs []string
	seen := make(map[string]bool)
	for _, pattern := range s.Dirs {
		var matches []string
		if err := walkPattern(l, ".", segments(pattern), &matches); err != nil {
			return nil, diag.Wrap(diag.IO, diag.Pos{}, err)
		}
		if len(matches) == 0 {
			return nil, diag.Errorf(diag.NoMatch, diag.Pos{}, "dirs pattern %q matched no directories", pattern)
		}
		for _, dir := range matches {
			if !seen[dir] && !excluded(dir, s.Exclude) {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs, nil
}

func segments(p string) []string {
	if p == "." {
		return nil
	}
	return strings.Split(p, "/")
}

// walkPattern appends the directories below dir that match pattern. Like go's ./..., directories matched by a
// wildcard are skipped when they are named testdata or vendor or start with "." or "_".
func walkPattern(l *listing, dir string, pattern []string, out *[]string) error {
	if len(pattern) == 0 {
		*out = append(*out, dir)
		return nil
	}
	segment, rest := pattern[0], pattern[1:]

	switch {
	case segment == "**":
		if err := walkPattern(l, dir, rest, out); err != nil {
			return err
		}
		children, err := wildcardChildren(l, dir)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := walkPattern(l, child, pattern, out); err != nil {
				return err
			}
		}
	case strings.ContainsAny(segment, `*?[\`):
		children, err := wildcardChildren(l, dir)
		if err != nil {
			return err
		}
		for _, child := range children {
			if ok, _ := path.Match(segment, path.Base(child)); ok {
				if err := walkPattern(l, child, rest, out); err != nil {
					return err
				}
			}
		}
	default:
		child := path.Join(dir, segment)
		// Lstat, so a directory that is a symbolic link is skipped like any other link.
		if info, err := fs.Lstat(l.fsys, child); err == nil && info.IsDir() {
			return walkPattern(l, child, rest, out)
		}
	}
	return nil
}

// wildcardChildren lists the subdirectories of dir that a wildcard may match.
func wildcardChildren(l *listing, dir string) ([]string, error) {
	entries, err := l.read(dir)
	if err != nil {
		return nil, err
	}
	var children []string
	for _, entry := range entries {
		if !entry.IsDir() || Ignored(entry.Name()) || l.boundary(path.Join(dir, entry.Name())) {
			continue
		}
		name := entry.Name()
		children = append(children, path.Join(dir, name))
	}
	return children, nil
}

// Ignored reports whether wildcards skip a directory with this name: like go's ./..., testdata, vendor, and
// names starting with "." or "_".
func Ignored(name string) bool {
	return name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// Boundary reports whether dir holds its own adder.yaml or go.mod, which makes it another configuration's or
// module's, so wildcards and the search for stale files stop there, as go's ./... stops at nested modules.
func Boundary(fsys fs.FS, dir string) bool {
	for _, name := range []string{config.FileName, "go.mod"} {
		if info, err := fs.Stat(fsys, path.Join(dir, name)); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func excluded(dir string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchSegments(segments(pattern), segments(dir)) {
			return true
		}
	}
	return false
}

// matchSegments reports whether a directory path matches a pattern, where "**" matches any number of segments.
func matchSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		for i := range len(name) + 1 {
			if matchSegments(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	ok, _ := path.Match(pattern[0], name[0])
	return ok && matchSegments(pattern[1:], name[1:])
}
