package imports

import "testing"

func TestName(t *testing.T) {
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
			if got := Name(tt.importPath); got != tt.want {
				t.Errorf("Name(%q) = %q, want %q", tt.importPath, got, tt.want)
			}
		})
	}
}

func TestPrune(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // empty means unchanged
	}{
		{
			name: "all used",
			src:  "package p\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nvar _, _ = fmt.Sprint, os.Exit\n",
		},
		{
			name: "one unused in a block",
			src:  "package p\n\nimport (\n\t\"fmt\"\n\t\"os\"\n\t\"strings\"\n)\n\nvar _, _ = fmt.Sprint, strings.Cut\n",
			want: "package p\n\nimport (\n\t\"fmt\"\n\t\"strings\"\n)\n\nvar _, _ = fmt.Sprint, strings.Cut\n",
		},
		{
			name: "first and last unused",
			src:  "package p\n\nimport (\n\t\"fmt\"\n\t\"os\"\n\t\"strings\"\n)\n\nvar _ = os.Exit\n",
			want: "package p\n\nimport (\n\t\"os\"\n)\n\nvar _ = os.Exit\n",
		},
		{
			name: "a whole group",
			src:  "package p\n\nimport (\n\t\"fmt\"\n\n\t\"example.com/m/a\"\n\t\"example.com/m/b\"\n)\n\nvar _ = fmt.Sprint\n",
			want: "package p\n\nimport (\n\t\"fmt\"\n)\n\nvar _ = fmt.Sprint\n",
		},
		{
			name: "a group between groups",
			src:  "package p\n\nimport (\n\t\"fmt\"\n\n\t\"example.com/m/a\"\n\n\t\"example.com/x/b\"\n)\n\nvar _, _ = fmt.Sprint, b.B\n",
			want: "package p\n\nimport (\n\t\"fmt\"\n\n\t\"example.com/x/b\"\n)\n\nvar _, _ = fmt.Sprint, b.B\n",
		},
		{
			name: "every import",
			src:  "// Package p is p.\npackage p\n\n// Imports.\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nvar X = 1\n",
			want: "// Package p is p.\npackage p\n\nvar X = 1\n",
		},
		{
			name: "single imports",
			src:  "package p\n\nimport \"fmt\"\nimport \"os\"\n\nvar _ = os.Exit\n",
			want: "package p\n\nimport \"os\"\n\nvar _ = os.Exit\n",
		},
		{
			name: "comments go with their import",
			src:  "package p\n\nimport (\n\t// fmt formats.\n\t\"fmt\" // trailing\n\t\"os\"\n)\n\nvar _ = os.Exit\n",
			want: "package p\n\nimport (\n\t\"os\"\n)\n\nvar _ = os.Exit\n",
		},
		{
			name: "explicit names",
			src:  "package p\n\nimport (\n\tstr \"strings\"\n\tyaml \"go.yaml.in/yaml/v3\"\n)\n\nvar _ = yaml.Marshal\n",
			want: "package p\n\nimport (\n\tyaml \"go.yaml.in/yaml/v3\"\n)\n\nvar _ = yaml.Marshal\n",
		},
		{
			name: "assumed names",
			src:  "package p\n\nimport (\n\t\"github.com/mattn/go-sqlite3\"\n\t\"go.yaml.in/yaml/v3\"\n\t\"gopkg.in/check.v1\"\n)\n\nvar _, _, _ = sqlite3.X, yaml.Y, check.Z\n",
		},
		{
			name: "blank, dot, and cgo imports are kept",
			src:  "package p\n\n// #include <stdio.h>\nimport \"C\"\n\nimport (\n\t. \"fmt\"\n\t_ \"embed\"\n)\n\nvar _ = Sprint\n",
		},
		{
			name: "a name used only by a local variable is kept",
			src:  "package p\n\nimport \"os\"\n\nfunc f(os struct{ X int }) int { return os.X }\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Prune([]byte(tt.src))
			if err != nil {
				t.Fatalf("Prune() error = %v", err)
			}
			want := tt.want
			if want == "" {
				want = tt.src
			}
			if string(got) != want {
				t.Errorf("Prune() =\n%s\nwant:\n%s", got, want)
			}
		})
	}

	if _, err := Prune([]byte("package")); err == nil {
		t.Error("Prune() of invalid Go succeeded")
	}
}
