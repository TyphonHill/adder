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
			var comment *ast.CommentGroup
			if !gen.Lparen.IsValid() {
				comment = gen.Specs[0].(*ast.ImportSpec).Comment
			}
			cuts = append(cuts, lines(file, src, gen.Pos(), gen.End(), gen.Doc, comment))
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
	name, importPath, ok := LocalName(spec)
	return !ok || importPath == "C" || used[name]
}

// LocalName returns the name a file refers to an import by, and the import's path. It reports false for an
// import a file cannot refer to by name: a blank, dot, or malformed one.
func LocalName(spec *ast.ImportSpec) (name, importPath string, ok bool) {
	importPath, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		return "", "", false
	}
	name = Name(importPath)
	if spec.Name != nil {
		name = spec.Name.Name
	}
	return name, importPath, name != "_" && name != "."
}

// span is a range of bytes in a file.
type span struct{ start, end int }

// lines returns the range from pos to end, extended to the doc comment above and the comment after, and to whole
// lines when nothing else shares them, such as the end of a block comment.
func lines(file *token.File, src []byte, pos, end token.Pos, doc, comment *ast.CommentGroup) span {
	if doc != nil {
		pos = doc.Pos()
	}
	if comment != nil {
		end = comment.End()
	}
	start, stop := file.Offset(pos), file.Offset(end)
	if lineStart := file.Offset(file.LineStart(file.Line(pos))); isBlank(src[lineStart:start]) {
		start = lineStart
	}
	lineEnd := len(src)
	if i := bytes.IndexByte(src[stop:], '\n'); i >= 0 {
		lineEnd = stop + i + 1
	}
	if isBlank(src[stop:lineEnd]) {
		stop = lineEnd
	}
	return span{start, stop}
}

func isBlank(b []byte) bool {
	return len(bytes.TrimSpace(b)) == 0
}
