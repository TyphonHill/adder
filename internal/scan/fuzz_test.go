package scan

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"typhonhill.dev/adder/internal/diag"
)

// FuzzDirectives checks that any doc comment line either parses as directives or is reported on its own line.
func FuzzDirectives(f *testing.F) {
	for _, seed := range []string{
		"adder:register",
		"adder:register name=s3 table=objects",
		`adder:register desc="Local disk, \"fast\"" name=disk`,
		`adder:register desc="open`,
		"adder:register name=a name=b",
		"adder:Register",
		" F does things.",
		"go:generate stringer",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, line string) {
		if strings.ContainsAny(line, "\r\n") {
			return
		}
		src := "package p\n\n//" + line + "\nfunc F() {}\n"
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
		if err != nil {
			return
		}
		fn, ok := file.Decls[0].(*ast.FuncDecl)
		if !ok || fn.Doc == nil {
			return
		}
		_, err = directives(fset, fn.Doc)
		for _, d := range diag.From(err) {
			if d.Code != diag.InvalidDirective || d.Pos.File != "p.go" || d.Pos.Line != 3 {
				t.Errorf("directives(%q) problem = %#v, want invalid-directive on line 3", line, d)
			}
		}
	})
}
