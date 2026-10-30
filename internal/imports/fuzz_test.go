package imports

import (
	"bytes"
	"go/format"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"testing"

	"golang.org/x/mod/module"
)

// FuzzPrune checks that pruning gofmt-formatted Go keeps it formatted, only removes imports, and is idempotent.
func FuzzPrune(f *testing.F) {
	for _, seed := range []string{
		"package p\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nvar _ = fmt.Sprint\n",
		"package p\n\nimport \"fmt\" // print\n\nfunc F() {}\n",
		"package p\n\nimport (\n\t// Doc.\n\tx \"example.com/y\"\n\n\t_ \"embed\"\n\t. \"strings\"\n)\n\nvar _ = Cut\n",
		"package p\n\n/*\n#include <stdio.h>\n*/\nimport \"C\"\n\nimport \"os\"\n",
		"package p\n\nimport \"example.com/client/v2\"\n\nfunc F(client int) { _ = client.X }\n",
		"package p\n\nimport (\n\t\"fmt\"\n)\n\nimport \"os\"\n",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		// format.Source also accepts fragments, but Prune works on whole files.
		if _, err := parser.ParseFile(token.NewFileSet(), "", src, parser.ParseComments); err != nil {
			return
		}
		formatted, err := format.Source(src)
		if err != nil {
			return
		}
		// Invalid import paths, such as empty ones, do not compile and gofmt formats them unstably.
		for _, p := range importPaths(t, src) {
			if p != "C" && module.CheckImportPath(p) != nil {
				return
			}
		}
		// gofmt is not idempotent on some inputs, such as an import block holding only a comment.
		if again, err := format.Source(formatted); err != nil || !bytes.Equal(again, formatted) {
			return
		}
		out, err := Prune(formatted)
		if err != nil {
			t.Fatalf("Prune() error = %v for formatted input:\n%s", err, formatted)
		}
		if again, err := format.Source(out); err != nil || string(again) != string(out) {
			t.Fatalf("Prune() output is not gofmt-formatted (error %v):\n%s", err, out)
		}
		before, after := importPaths(t, formatted), importPaths(t, out)
		for _, p := range after {
			if !slices.Contains(before, p) {
				t.Errorf("Prune() added import %q", p)
			}
		}
		if twice, err := Prune(out); err != nil || string(twice) != string(out) {
			t.Errorf("Prune() is not idempotent (error %v):\n%s", err, twice)
		}
	})
}

func importPaths(t *testing.T, src []byte) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("output does not parse: %v\n%s", err, src)
	}
	var paths []string
	for _, spec := range f.Imports {
		p, _ := strconv.Unquote(spec.Path.Value)
		paths = append(paths, p)
	}
	return paths
}
