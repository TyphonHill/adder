// Package imports reasons about the import declarations of Go files.
package imports

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Name returns the package name goimports assumes for an import path without an alias: the last element,
// skipping a major version suffix such as v2, without a go- prefix, and cut at the first character that cannot be
// in an identifier.
func Name(importPath string) string {
	base := path.Base(importPath)
	if strings.HasPrefix(base, "v") {
		if _, err := strconv.Atoi(base[1:]); err == nil {
			if dir := path.Dir(importPath); dir != "." {
				base = path.Base(dir)
			}
		}
	}
	base = strings.TrimPrefix(base, "go-")
	if i := strings.IndexFunc(base, func(r rune) bool { return !identifierRune(r) }); i >= 0 {
		base = base[:i]
	}
	return base
}

func identifierRune(r rune) bool {
	return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' || r == '_' ||
		r >= utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

// Prune removes the imports a gofmt-formatted Go file never refers to, with their comments, and formats the
// result. A package is referred to by its explicit name, or by the name Name assumes, so an import whose package
// name differs from its path needs an explicit name. Blank, dot, and cgo imports are always kept.
func Prune(src []byte) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	used := make(map[string]bool)
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				used[id.Name] = true
			}
		}
		return true
	})

	file := fset.File(f.Pos())
	var cuts []span
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		var unused []span
		for _, spec := range gen.Specs {
			if is := spec.(*ast.ImportSpec); !isUsed(is, used) {
				unused = append(unused, lines(file, src, is.Pos(), is.End(), is.Doc, is.Comment))
			}
		}
		switch {
		case len(unused) == 0:
		case len(unused) == len(gen.Specs):
			cuts = append(cuts, lines(file, src, gen.Pos(), gen.End(), gen.Doc, nil))
		default:
			cuts = append(cuts, unused...)
		}
	}
	if len(cuts) == 0 {
		return src, nil
	}

	var out bytes.Buffer
	last := 0
	for _, c := range cuts {
		out.Write(src[last:c.start])
		last = c.end
	}
	out.Write(src[last:])
	return format.Source(out.Bytes())
}

func isUsed(spec *ast.ImportSpec, used map[string]bool) bool {
	importPath, err := strconv.Unquote(spec.Path.Value)
	if err != nil || importPath == "C" {
		return true
	}
	name := Name(importPath)
	if spec.Name != nil {
		name = spec.Name.Name
	}
	return name == "_" || name == "." || used[name]
}

// span is a range of bytes in a file.
type span struct{ start, end int }

// lines returns the whole lines from pos to end, extended to the doc comment above and the comment after.
func lines(file *token.File, src []byte, pos, end token.Pos, doc, comment *ast.CommentGroup) span {
	if doc != nil {
		pos = doc.Pos()
	}
	if comment != nil {
		end = comment.End()
	}
	start := file.Offset(file.LineStart(file.Line(pos)))
	stop := file.Offset(end)
	if i := bytes.IndexByte(src[stop:], '\n'); i >= 0 {
		stop += i + 1
	} else {
		stop = len(src)
	}
	return span{start, stop}
}
