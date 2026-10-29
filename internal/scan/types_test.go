package scan

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"testing"
)

func TestAssumedName(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		want       string
	}{
		{name: "standard library", importPath: "context", want: "context"},
		{name: "last element", importPath: "go.temporal.io/sdk/workflow", want: "workflow"},
		{name: "major version suffix", importPath: "example.com/client/v2", want: "client"},
		{name: "gopkg.in version", importPath: "gopkg.in/yaml.v3", want: "yaml"},
		{name: "go- prefix", importPath: "github.com/mattn/go-sqlite3", want: "sqlite3"},
		{name: "hyphenated name", importPath: "example.com/my-lib", want: "my"},
		{name: "v without digits", importPath: "example.com/vault", want: "vault"},
		{name: "version alone", importPath: "v2", want: "v2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := assumedName(tt.importPath); got != tt.want {
				t.Errorf("assumedName(%q) = %q, want %q", tt.importPath, got, tt.want)
			}
		})
	}
}

func TestFileImports(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want map[string]string
	}{
		{
			name: "default and aliased names",
			src:  "package p\n\nimport (\n\t\"context\"\n\twf \"go.temporal.io/sdk/workflow\"\n)\n",
			want: map[string]string{"context": "context", "wf": "go.temporal.io/sdk/workflow"},
		},
		{
			name: "blank and dot imports are left out",
			src:  "package p\n\nimport (\n\t_ \"embed\"\n\t. \"strings\"\n)\n",
			want: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := parser.ParseFile(token.NewFileSet(), "p.go", tt.src, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			if got := fileImports(f); !maps.Equal(got, tt.want) {
				t.Errorf("fileImports() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQualifiedType(t *testing.T) {
	imports := map[string]string{"wf": "go.temporal.io/sdk/workflow", "context": "context"}
	tests := []struct {
		name string
		expr string
		want string
	}{
		{name: "builtin", expr: "string", want: "string"},
		{name: "imported", expr: "context.Context", want: "context.Context"},
		{name: "aliased", expr: "wf.Context", want: "go.temporal.io/sdk/workflow.Context"},
		{name: "unknown package kept as written", expr: "other.Type", want: "other.Type"},
		{name: "pointer", expr: "*wf.Context", want: "*go.temporal.io/sdk/workflow.Context"},
		{name: "slice", expr: "[]wf.Context", want: "[]go.temporal.io/sdk/workflow.Context"},
		{name: "array", expr: "[4]byte", want: "[4]byte"},
		{name: "map", expr: "map[string]wf.Context", want: "map[string]go.temporal.io/sdk/workflow.Context"},
		{name: "variadic", expr: "...wf.Context", want: "...go.temporal.io/sdk/workflow.Context"},
		{name: "channel", expr: "chan int", want: "chan int"},
		{name: "receive channel", expr: "<-chan wf.Context", want: "<-chan go.temporal.io/sdk/workflow.Context"},
		{name: "send channel", expr: "chan<- int", want: "chan<- int"},
		{name: "generic", expr: "List[wf.Context]", want: "List[go.temporal.io/sdk/workflow.Context]"},
		{name: "generic with several arguments", expr: "Map[string, int]", want: "Map[string,int]"},
		{name: "parenthesized", expr: "(*int)", want: "(*int)"},
		{name: "function literal as written", expr: "func(int) error", want: "func(int) error"},
		{name: "struct literal as written", expr: "struct{}", want: "struct{}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "package p\n\nvar _ " + tt.expr + "\n"
			if tt.expr[0] == '.' {
				src = "package p\n\nfunc _(x " + tt.expr + ") {}\n"
			}
			f, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0)
			if err != nil {
				t.Fatal(err)
			}
			expr := typeOf(f)
			if got := qualifiedType(expr, imports); got != tt.want {
				t.Errorf("qualifiedType(%s) = %q, want %q", tt.expr, got, tt.want)
			}
		})
	}
}

// typeOf returns the type in a file with one declaration: a variable's type or a function's first parameter type.
func typeOf(f *ast.File) ast.Expr {
	switch d := f.Decls[0].(type) {
	case *ast.GenDecl:
		return d.Specs[0].(*ast.ValueSpec).Type
	case *ast.FuncDecl:
		return d.Type.Params.List[0].Type
	}
	return nil
}

func TestSameType(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{name: "identical", a: "[]string", b: "[]string", want: true},
		{name: "whitespace ignored", a: "func(int) error", b: "func(int)error", want: true},
		{name: "different", a: "[]string", b: "[]int", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameType(tt.a, tt.b); got != tt.want {
				t.Errorf("sameType(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
