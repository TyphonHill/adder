package scan

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"typhonhill.dev/adder/internal/diag"
)

func TestDirectives(t *testing.T) {
	tests := []struct {
		name     string
		comment  string
		want     map[string]map[string]string
		wantLine int // when set, a problem is expected on this line of the comment
	}{
		{name: "no comment", comment: "", want: nil},
		{name: "no directives", comment: "// F does things.", want: nil},
		{name: "other tools' directives are ignored", comment: "//go:generate stringer\n//nolint:errcheck", want: nil},
		{name: "without arguments", comment: "//adder:register", want: map[string]map[string]string{"register": {}}},
		{name: "plain values", comment: "//adder:register name=s3 table=objects", want: map[string]map[string]string{"register": {"name": "s3", "table": "objects"}}},
		{name: "quoted values", comment: `//adder:register desc="Local disk, \"fast\"" name=disk`, want: map[string]map[string]string{"register": {"desc": `Local disk, "fast"`, "name": "disk"}}},
		{name: "empty value", comment: "//adder:register name=", want: map[string]map[string]string{"register": {"name": ""}}},
		{name: "extra spacing", comment: "//adder:register   name=s3\ttable=t  ", want: map[string]map[string]string{"register": {"name": "s3", "table": "t"}}},
		{
			name:    "several directives",
			comment: "// Doc.\n//\n//adder:register name=s3\n//adder:route path=/s3",
			want:    map[string]map[string]string{"register": {"name": "s3"}, "route": {"path": "/s3"}},
		},
		{name: "uppercase name", comment: "//adder:Register", wantLine: 1},
		{name: "repeated directive", comment: "//adder:register\n//adder:register", wantLine: 2},
		{name: "argument without a value", comment: "//adder:register name", wantLine: 1},
		{name: "invalid argument key", comment: "//adder:register 9name=x", wantLine: 1},
		{name: "repeated argument", comment: "//adder:register name=a name=b", wantLine: 1},
		{name: "unterminated quote", comment: `//adder:register desc="open`, wantLine: 1},
		{name: "text after a quoted value", comment: `//adder:register desc="a"b`, wantLine: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const firstLine = 3 // the comment starts on this line of the source below
			src := "package p\n\n" + tt.comment + "\nfunc F() {}\n"
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}

			got, err := directives(fset, f.Decls[0].(*ast.FuncDecl).Doc)
			if tt.wantLine != 0 {
				list := diag.From(err)
				want := diag.Pos{File: "p.go", Line: firstLine + tt.wantLine - 1, Column: 1}
				if len(list) != 1 || list[0].Code != diag.InvalidDirective || list[0].Pos != want {
					t.Fatalf("directives() error = %#v, want an invalid-directive problem at %v", list, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("directives() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("directives() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
