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

// Item is a matched declaration.
type Item struct {
	Name       string  // declared name
	Kind       string  // "func" or "method"
	Receiver   string  // receiver type name, empty for plain functions
	Package    string  // package name from the file's package clause
	ImportPath string  // the package's import path, empty when the repository has no go.mod
	Dir        string  // slash-separated directory relative to the root
	File       string  // slash-separated file path relative to the root
	Line       int     // line of the declaration
	Doc        string  // doc comment without directives
	Signature  string  // parameters and results as written, such as "(ctx context.Context) error"
	Params     []Param // parameters, one per name
	Results    []Param // results, one per name
}

// Param is a parameter or result of a function.
type Param struct {
	Name string // empty for unnamed parameters
	Type string // as written, such as "context.Context"
}

// Run returns the declarations in fsys selected by s, sorted by name. Paths in fsys are relative to the repository
// root, and module is the repository's module path, or empty when it has none.
// Files adder generated are skipped, so outputs never feed back into scans; files from other generators are scanned.
// Problems with the scan itself carry no position; problems in a file carry that file's path within fsys.
func Run(ctx context.Context, fsys fs.FS, module string, s config.Scan) ([]Item, error) {
	dirs, err := expandDirs(fsys, s)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	var items []Item
	for _, dir := range dirs {
		found, err := scanDir(ctx, fsys, fset, module, dir, s)
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

func scanDir(ctx context.Context, fsys fs.FS, fset *token.FileSet, module, dir string, s config.Scan) ([]Item, error) {
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

		imports := fileImports(parsed)
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !matches(fn, imports, s.Funcs) {
				continue
			}
			items = append(items, funcItem(fset, fn, Item{
				Package:    parsed.Name.Name,
				ImportPath: importPath(module, dir),
				Dir:        dir,
				File:       file,
			}))
		}
	}
	return items, nil
}

// funcItem fills in the fields of item that describe fn.
func funcItem(fset *token.FileSet, fn *ast.FuncDecl, item Item) Item {
	item.Name = fn.Name.Name
	item.Kind = config.KindFunc
	if fn.Recv != nil {
		item.Kind = config.KindMethod
		item.Receiver = receiverName(fn)
	}
	item.Line = fset.Position(fn.Pos()).Line
	if fn.Doc != nil {
		item.Doc = strings.TrimSpace(fn.Doc.Text())
	}
	item.Signature = strings.TrimPrefix(written(fn.Type), "func")
	item.Params = params(fn.Type.Params)
	item.Results = params(fn.Type.Results)
	return item
}

func params(list *ast.FieldList) []Param {
	if list == nil {
		return nil
	}
	var out []Param
	for _, field := range list.List {
		typ := written(field.Type)
		if len(field.Names) == 0 {
			out = append(out, Param{Type: typ})
			continue
		}
		for _, name := range field.Names {
			out = append(out, Param{Name: name.Name, Type: typ})
		}
	}
	return out
}

func importPath(module, dir string) string {
	if module == "" {
		return ""
	}
	if dir == "." {
		return module
	}
	return module + "/" + dir
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

func matches(fn *ast.FuncDecl, imports map[string]string, m config.FuncMatch) bool {
	switch {
	case m.Exported && !fn.Name.IsExported():
		return false
	case m.Name.Regexp != nil && !m.Name.MatchString(fn.Name.Name):
		return false
	case m.Kind == config.KindFunc && fn.Recv != nil, m.Kind == config.KindMethod && fn.Recv == nil:
		return false
	case m.Receiver != "" && (fn.Recv == nil || receiverName(fn) != m.Receiver):
		return false
	case m.FirstParam != "" && firstParamName(fn) != m.FirstParam:
		return false
	case m.FirstParamType != "":
		first := firstParam(fn)
		return first != nil && sameType(qualifiedType(first.Type, imports), m.FirstParamType)
	}
	return true
}

func firstParam(fn *ast.FuncDecl) *ast.Field {
	if params := fn.Type.Params.List; len(params) > 0 {
		return params[0]
	}
	return nil
}

func firstParamName(fn *ast.FuncDecl) string {
	if first := firstParam(fn); first != nil && len(first.Names) > 0 {
		return first.Names[0].Name
	}
	return ""
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
