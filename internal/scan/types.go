package scan

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"
	"strconv"
	"strings"

	"typhonhill.dev/adder/internal/imports"
)

// fileImports maps the names a file refers to its imports by to their import paths.
func fileImports(f *ast.File) map[string]string {
	byName := make(map[string]string, len(f.Imports))
	for _, spec := range f.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := imports.Name(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name != "_" && name != "." {
			byName[name] = importPath
		}
	}
	return byName
}

// qualifiedType writes a type expression the way Go does, with each package name replaced by its import path.
func qualifiedType(expr ast.Expr, imports map[string]string) string {
	var b strings.Builder
	writeQualified(&b, expr, imports)
	return b.String()
}

func writeQualified(b *strings.Builder, expr ast.Expr, imports map[string]string) {
	switch t := expr.(type) {
	case *ast.Ident:
		b.WriteString(t.Name)
	case *ast.SelectorExpr:
		// In a type, the selector's operand is always a package name.
		pkg := written(t.X)
		if importPath, ok := imports[pkg]; ok {
			pkg = importPath
		}
		b.WriteString(pkg)
		b.WriteString(".")
		b.WriteString(t.Sel.Name)
	case *ast.StarExpr:
		b.WriteString("*")
		writeQualified(b, t.X, imports)
	case *ast.ArrayType:
		b.WriteString("[")
		if t.Len != nil {
			b.WriteString(written(t.Len))
		}
		b.WriteString("]")
		writeQualified(b, t.Elt, imports)
	case *ast.MapType:
		b.WriteString("map[")
		writeQualified(b, t.Key, imports)
		b.WriteString("]")
		writeQualified(b, t.Value, imports)
	case *ast.Ellipsis:
		b.WriteString("...")
		writeQualified(b, t.Elt, imports)
	case *ast.ChanType:
		switch t.Dir {
		case ast.RECV:
			b.WriteString("<-chan ")
		case ast.SEND:
			b.WriteString("chan<- ")
		default:
			b.WriteString("chan ")
		}
		writeQualified(b, t.Value, imports)
	case *ast.IndexExpr:
		writeQualified(b, t.X, imports)
		b.WriteString("[")
		writeQualified(b, t.Index, imports)
		b.WriteString("]")
	case *ast.IndexListExpr:
		writeQualified(b, t.X, imports)
		b.WriteString("[")
		for i, index := range t.Indices {
			if i > 0 {
				b.WriteString(",")
			}
			writeQualified(b, index, imports)
		}
		b.WriteString("]")
	case *ast.ParenExpr:
		b.WriteString("(")
		writeQualified(b, t.X, imports)
		b.WriteString(")")
	default:
		// Function, struct, and interface literals are rare as parameter types; they are matched as written.
		b.WriteString(written(expr))
	}
}

// written returns a syntax node exactly as gofmt would print it.
func written(node ast.Node) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), node); err != nil {
		return ""
	}
	return buf.String()
}

// sameType compares type strings ignoring whitespace, so "func(int) error" matches "func(int)error".
func sameType(a, b string) bool {
	return strings.Join(strings.Fields(a), "") == strings.Join(strings.Fields(b), "")
}
