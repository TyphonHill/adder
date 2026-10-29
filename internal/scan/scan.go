// Package scan finds Go declarations that match a configured filter.
package scan

import (
	"cmp"
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/TyphonHill/adder/internal/config"
	"github.com/TyphonHill/adder/internal/diag"
	"github.com/TyphonHill/adder/internal/header"
)

// Item is a matched function or method declaration.
type Item struct {
	Name     string // declared name
	Receiver string // receiver type name, empty for plain functions
	Package  string // package name from the file's package clause
	Dir      string // slash-separated directory relative to the root
	File     string // slash-separated file path relative to the root
}

// Run returns the declarations in fsys selected by s, sorted by name. Paths in fsys are relative to the repository root.
// Files adder generated are skipped, so outputs never feed back into scans; files from other generators are scanned.
// Problems with the scan itself carry no position; problems in a file carry that file's path within fsys.
func Run(ctx context.Context, fsys fs.FS, s config.Scan) ([]Item, error) {
	dirs, err := expandDirs(fsys, s)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	var items []Item
	for _, dir := range dirs {
		found, err := scanDir(ctx, fsys, fset, dir, s)
		if err != nil {
			return nil, err
		}
		items = append(items, found...)
	}

	slices.SortFunc(items, func(a, b Item) int {
		return cmp.Or(
			cmp.Compare(a.Name, b.Name),
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Receiver, b.Receiver),
		)
	})
	return items, nil
}

// expandDirs resolves the scan's glob patterns to unique directories.
func expandDirs(fsys fs.FS, s config.Scan) ([]string, error) {
	var dirs []string
	seen := make(map[string]bool)
	for _, pattern := range s.Dirs {
		matches, err := fs.Glob(fsys, pattern)
		if err != nil {
			return nil, diag.Errorf(diag.InvalidValue, diag.Pos{}, "dirs pattern %q: %v", pattern, err)
		}

		matched := false
		for _, match := range matches {
			info, err := fs.Stat(fsys, match)
			if err != nil || !info.IsDir() {
				continue
			}
			matched = true
			if !seen[match] {
				seen[match] = true
				dirs = append(dirs, match)
			}
		}
		if !matched {
			return nil, diag.Errorf(diag.NoMatch, diag.Pos{}, "dirs pattern %q matched no directories", pattern)
		}
	}
	return dirs, nil
}

func scanDir(ctx context.Context, fsys fs.FS, fset *token.FileSet, dir string, s config.Scan) ([]Item, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, diag.Wrap(diag.IO, diag.Pos{File: dir}, err)
	}

	var items []Item
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := entry.Name()
		if !entry.Type().IsRegular() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if !s.IncludeTests && strings.HasSuffix(name, "_test.go") {
			continue
		}

		file := path.Join(dir, name)
		src, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, diag.Wrap(diag.IO, diag.Pos{File: file}, err)
		}
		parsed, err := parser.ParseFile(fset, file, src, parser.SkipObjectResolution|parser.ParseComments)
		if err != nil {
			return nil, syntaxError(file, err)
		}
		if _, generated := header.Source(parsed); generated {
			continue
		}

		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !matches(fn, s.Funcs) {
				continue
			}
			items = append(items, Item{
				Name:     fn.Name.Name,
				Receiver: receiverName(fn),
				Package:  parsed.Name.Name,
				Dir:      dir,
				File:     file,
			})
		}
	}
	return items, nil
}

// syntaxError reports the first Go syntax error in a file with its position.
func syntaxError(file string, err error) error {
	if list, ok := errors.AsType[scanner.ErrorList](err); ok && len(list) > 0 {
		first := list[0]
		return &diag.Diagnostic{
			Code:    diag.GoSyntax,
			Message: first.Msg,
			Pos:     diag.Pos{File: file, Line: first.Pos.Line, Column: first.Pos.Column},
			Err:     err,
		}
	}
	return diag.Wrap(diag.GoSyntax, diag.Pos{File: file}, err)
}

func matches(fn *ast.FuncDecl, m config.FuncMatch) bool {
	if m.Exported && !fn.Name.IsExported() {
		return false
	}
	if m.Name.Regexp != nil && !m.Name.MatchString(fn.Name.Name) {
		return false
	}
	if m.FirstParam != "" && firstParamName(fn) != m.FirstParam {
		return false
	}
	return true
}

func firstParamName(fn *ast.FuncDecl) string {
	params := fn.Type.Params.List
	if len(params) == 0 || len(params[0].Names) == 0 {
		return ""
	}
	return params[0].Names[0].Name
}

func receiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	switch generic := expr.(type) {
	case *ast.IndexExpr:
		expr = generic.X
	case *ast.IndexListExpr:
		expr = generic.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}
