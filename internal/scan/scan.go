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
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"typhonhill.dev/adder/internal/config"
	"typhonhill.dev/adder/internal/diag"
	"typhonhill.dev/adder/internal/header"
)

// Item is a matched declaration.
type Item struct {
	Name       string  // declared name
	Kind       string  // "func", "method", "struct", "interface", "type", "const", "var", or "file"
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
	Fields     []Field // struct fields, one per name
	Type       string  // for constants and variables, the type as written, repeated through a const group
	Value      string  // for constants and variables, the value as written
	Iota       int     // for constants, the position in their const group, which is their value of iota
	Pointer    bool    // for types selected by implements, true when only a pointer to the type implements it

	// Directives holds the arguments of each //adder: directive in the doc comment, by directive name.
	Directives map[string]map[string]string
}

// Param is a parameter or result of a function.
type Param struct {
	Name string // empty for unnamed parameters
	Type string // as written, such as "context.Context"
}

// Field is a field of a struct.
type Field struct {
	Name     string // for embedded fields, the type name without its package, like Go uses
	Type     string // as written
	Tag      string // the tag without its quotes, such as `json:"name"`
	Embedded bool
}

// Run returns the declarations in fsys selected by s, sorted by name. Paths in fsys are relative to the repository
// root, and module is the repository's module path, or empty when it has none.
// Files adder generated are skipped, so outputs never feed back into scans; files from other generators are scanned.
// Problems with the scan itself carry no position; problems in a file carry that file's path within fsys.
func Run(ctx context.Context, fsys fs.FS, module string, s config.Scan) ([]Item, error) {
	l := newListing(fsys)
	dirs, err := expandDirs(l, s)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	var items []Item
	for _, dir := range dirs {
		found, err := scanDir(ctx, l, fset, module, dir, s)
		if err != nil {
			return nil, err
		}
		items = append(items, found...)
	}

	sortItems(items)
	return items, nil
}

// sortItems orders items by name, file, and receiver, the same for every scan.
func sortItems(items []Item) {
	slices.SortFunc(items, func(a, b Item) int {
		return cmp.Or(
			cmp.Compare(a.Name, b.Name),
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Receiver, b.Receiver),
		)
	})
}

func scanDir(ctx context.Context, l *listing, fset *token.FileSet, module, dir string, s config.Scan) ([]Item, error) {
	fsys := l.fsys
	entries, err := l.read(dir)
	if err != nil {
		return nil, diag.Wrap(diag.IO, diag.Pos{File: dir}, err)
	}
	if s.Files != nil {
		return fileItems(ctx, fsys, dir, entries, *s.Files)
	}
	pkgPath := l.importPath(module, dir)

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
		ok, err := buildable(fset, parsed, s.BuildTags)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		base := Item{Package: parsed.Name.Name, ImportPath: pkgPath, Dir: dir, File: file}
		found, err := declItems(fset, parsed, s, base, nil)
		if err != nil {
			return nil, err
		}
		items = append(items, found...)
	}
	return items, nil
}

// declItems returns the declarations in file that the scan selects. Typed applies the filters that compare
// types, and is nil for scans that only parse.
func declItems(fset *token.FileSet, file *ast.File, s config.Scan, base Item, typed *typeFilter) ([]Item, error) {
	switch {
	case s.Types != nil:
		return typeItems(fset, file, *s.Types, base, typed)
	case s.Values != nil:
		return valueItems(fset, file, *s.Values, base, typed)
	default:
		return funcItems(fset, file, s.Funcs, base, typed)
	}
}

func funcItems(fset *token.FileSet, file *ast.File, m config.FuncMatch, base Item, typed *typeFilter) ([]Item, error) {
	imports := fileImports(file)
	var items []Item
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !matches(fn, imports, m, typed) {
			continue
		}
		found, err := directives(fset, fn.Doc)
		if err != nil {
			return nil, err
		}
		if m.Directive != "" && found[m.Directive] == nil {
			continue
		}
		item := funcItem(fset, fn, base)
		item.Directives = found
		items = append(items, item)
	}
	return items, nil
}

func typeItems(fset *token.FileSet, file *ast.File, m config.TypeMatch, base Item, typed *typeFilter) ([]Item, error) {
	var items []Item
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts := spec.(*ast.TypeSpec)
			kind := typeKind(ts)
			switch {
			case m.Exported && !ts.Name.IsExported(),
				m.Name.Regexp != nil && !m.Name.MatchString(ts.Name.Name),
				m.Kind != "" && m.Kind != kind:
				continue
			}

			// A lone type declaration's comment belongs to the declaration; in a group, to each spec.
			doc := ts.Doc
			if doc == nil && !gen.Lparen.IsValid() {
				doc = gen.Doc
			}
			found, err := directives(fset, doc)
			if err != nil {
				return nil, err
			}
			if m.Directive != "" && found[m.Directive] == nil {
				continue
			}
			pointer := false
			if m.Implements != "" {
				var ok bool
				if ok, pointer = typed.implements(ts); !ok {
					continue
				}
			}

			item := base
			item.Name = ts.Name.Name
			item.Kind = kind
			item.Pointer = pointer
			item.Line = fset.Position(ts.Pos()).Line
			if doc != nil {
				item.Doc = strings.TrimSpace(doc.Text())
			}
			if st, ok := ts.Type.(*ast.StructType); ok && !ts.Assign.IsValid() {
				item.Fields = structFields(st)
			}
			item.Directives = found
			items = append(items, item)
		}
	}
	return items, nil
}

// fileItems lists the files in dir that the scan selects, skipping hidden files, links, and files adder generated.
func fileItems(ctx context.Context, fsys fs.FS, dir string, entries []fs.DirEntry, m config.FileMatch) ([]Item, error) {
	var items []Item
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := entry.Name()
		if !entry.Type().IsRegular() || strings.HasPrefix(name, ".") || (m.Name.Regexp != nil && !m.Name.MatchString(name)) {
			continue
		}
		file := path.Join(dir, name)
		if header.Supported(name) {
			content, err := fs.ReadFile(fsys, file)
			if err != nil {
				return nil, diag.Wrap(diag.IO, diag.Pos{File: file}, err)
			}
			if _, generated := header.Parse(name, content); generated {
				continue
			}
		}
		items = append(items, Item{Name: name, Kind: config.KindFile, Dir: dir, File: file})
	}
	return items, nil
}

func valueItems(fset *token.FileSet, file *ast.File, m config.ValueMatch, base Item, typed *typeFilter) ([]Item, error) {
	var items []Item
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
			continue
		}
		kind := gen.Tok.String()
		if m.Kind != "" && m.Kind != kind {
			continue
		}
		// A group's directives apply to every name in it; a name's own directive of the same name replaces them.
		var group map[string]map[string]string
		if gen.Lparen.IsValid() {
			var err error
			if group, err = directives(fset, gen.Doc); err != nil {
				return nil, err
			}
		}

		var repeated string // the type a const without its own type and value repeats
		for i, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			typ := ""
			if vs.Type != nil {
				typ = written(vs.Type)
			}
			if kind == config.KindConst {
				if vs.Type == nil && len(vs.Values) == 0 {
					typ = repeated
				} else {
					repeated = typ
				}
			}

			doc := vs.Doc
			if doc == nil && !gen.Lparen.IsValid() {
				doc = gen.Doc
			}
			own, err := directives(fset, doc)
			if err != nil {
				return nil, err
			}
			found := own
			if len(group) > 0 {
				found = maps.Clone(group)
				maps.Copy(found, own)
			}

			for j, name := range vs.Names {
				switch {
				case name.Name == "_",
					m.Exported && !name.IsExported(),
					m.Name.Regexp != nil && !m.Name.MatchString(name.Name),
					m.Type != "" && !typed.valueTypeIs(name, m.Type, typ),
					m.Directive != "" && found[m.Directive] == nil:
					continue
				}
				item := base
				item.Name = name.Name
				item.Kind = kind
				item.Line = fset.Position(name.Pos()).Line
				if doc != nil {
					item.Doc = strings.TrimSpace(doc.Text())
				}
				item.Type = typ
				if len(vs.Values) == len(vs.Names) {
					item.Value = written(vs.Values[j])
				}
				if kind == config.KindConst {
					item.Iota = i
				}
				item.Directives = found
				items = append(items, item)
			}
		}
	}
	return items, nil
}

func typeKind(ts *ast.TypeSpec) string {
	if ts.Assign.IsValid() {
		return config.KindType
	}
	switch ts.Type.(type) {
	case *ast.StructType:
		return config.KindStruct
	case *ast.InterfaceType:
		return config.KindInterface
	}
	return config.KindType
}

func structFields(st *ast.StructType) []Field {
	var fields []Field
	for _, f := range st.Fields.List {
		field := Field{Type: written(f.Type)}
		if f.Tag != nil {
			field.Tag, _ = strconv.Unquote(f.Tag.Value)
		}
		if len(f.Names) == 0 {
			field.Name = embeddedName(f.Type)
			field.Embedded = true
			fields = append(fields, field)
			continue
		}
		for _, name := range f.Names {
			field.Name = name.Name
			fields = append(fields, field)
		}
	}
	return fields
}

// embeddedName is the field name Go gives an embedded type: its type name without pointer, package, or type arguments.
func embeddedName(expr ast.Expr) string {
	for {
		switch t := expr.(type) {
		case *ast.StarExpr:
			expr = t.X
		case *ast.SelectorExpr:
			return t.Sel.Name
		case *ast.IndexExpr:
			expr = t.X
		case *ast.IndexListExpr:
			expr = t.X
		case *ast.Ident:
			return t.Name
		default:
			return written(expr)
		}
	}
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

func matches(fn *ast.FuncDecl, imports map[string]string, m config.FuncMatch, typed *typeFilter) bool {
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
	case m.FirstParamType != "" && typed != nil:
		return typed.firstParamIs(fn, m.FirstParamType)
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
