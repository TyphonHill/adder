package scan

import (
	"io/fs"
	"path"
	"strings"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/diag"
)

// expandDirs resolves the scan's directory patterns to unique directories, leaving out excluded ones.
func expandDirs(fsys fs.FS, s config.Scan) ([]string, error) {
	var dirs []string
	seen := make(map[string]bool)
	for _, pattern := range s.Dirs {
		var matches []string
		if err := walkPattern(fsys, ".", segments(pattern), &matches); err != nil {
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
func walkPattern(fsys fs.FS, dir string, pattern []string, out *[]string) error {
	if len(pattern) == 0 {
		*out = append(*out, dir)
		return nil
	}
	segment, rest := pattern[0], pattern[1:]

	switch {
	case segment == "**":
		if err := walkPattern(fsys, dir, rest, out); err != nil {
			return err
		}
		children, err := wildcardChildren(fsys, dir)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := walkPattern(fsys, child, pattern, out); err != nil {
				return err
			}
		}
	case strings.ContainsAny(segment, `*?[\`):
		children, err := wildcardChildren(fsys, dir)
		if err != nil {
			return err
		}
		for _, child := range children {
			if ok, _ := path.Match(segment, path.Base(child)); ok {
				if err := walkPattern(fsys, child, rest, out); err != nil {
					return err
				}
			}
		}
	default:
		child := path.Join(dir, segment)
		// Lstat, so a directory that is a symbolic link is skipped like any other link.
		if info, err := fs.Lstat(fsys, child); err == nil && info.IsDir() {
			return walkPattern(fsys, child, rest, out)
		}
	}
	return nil
}

// wildcardChildren lists the subdirectories of dir that a wildcard may match.
func wildcardChildren(fsys fs.FS, dir string) ([]string, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var children []string
	for _, entry := range entries {
		if !entry.IsDir() || Ignored(entry.Name()) {
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
