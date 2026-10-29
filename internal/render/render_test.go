package render

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/TyphonHill/adder/internal/diag"
)

func templates(content string) fstest.MapFS {
	return fstest.MapFS{"t.tmpl": &fstest.MapFile{Data: []byte(content)}}
}

func TestFile(t *testing.T) {
	data := map[string]any{"Names": []string{"b", "a"}}
	tests := []struct {
		name     string
		template string
		outPath  string
		want     string
	}{
		{
			name:     "go output is formatted",
			template: "package x\nvar   Names = []string{ {{- range .Names }}\"{{ . }}\",{{ end -}} }\n",
			outPath:  "x.go",
			want:     "package x\n\nvar Names = []string{\"b\", \"a\"}\n",
		},
		{
			name:     "other output is left alone",
			template: "{{ range sortAlpha .Names }}{{ . }}   {{ end }}",
			outPath:  "x.txt",
			want:     "a   b   ",
		},
		{
			name:     "crlf endings are normalized",
			template: "first\r\n{{ len .Names }}\r\n",
			outPath:  "x.txt",
			want:     "first\n2\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := File(templates(tt.template), "t.tmpl", tt.outPath, data)
			if err != nil {
				t.Fatalf("File() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("File() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFileErrors(t *testing.T) {
	data := map[string]any{"Names": []string{"a"}, "Module": "example.com/x"}
	tests := []struct {
		name     string
		fsys     fstest.MapFS
		outPath  string
		want     diag.Diagnostic
		wantHint string
	}{
		{
			name:    "missing template",
			fsys:    fstest.MapFS{},
			outPath: "x.txt",
			want:    diag.Diagnostic{Code: diag.IO, Pos: diag.Pos{File: "t.tmpl"}},
		},
		{
			name:    "syntax",
			fsys:    templates("line one\n{{ .Names "),
			outPath: "x.txt",
			want:    diag.Diagnostic{Code: diag.TemplateSyntax, Pos: diag.Pos{File: "t.tmpl", Line: 2}},
		},
		{
			name:     "missing key",
			fsys:     templates("ok\n  {{ .Nmes }}"),
			outPath:  "x.txt",
			want:     diag.Diagnostic{Code: diag.TemplateExec, Pos: diag.Pos{File: "t.tmpl", Line: 2, Column: 5}, Message: `<.Nmes>: map has no entry for key "Nmes"`},
			wantHint: "available fields: Module, Names",
		},
		{
			name:     "invalid go",
			fsys:     templates("package x\n\nfunc {{ index .Names 0 }}( {\n"),
			outPath:  "x.go",
			want:     diag.Diagnostic{Code: diag.InvalidOutput, Pos: diag.Pos{File: "t.tmpl"}},
			wantHint: "the rendered line is: func a( {",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := File(tt.fsys, "t.tmpl", tt.outPath, data)
			list := diag.From(err)
			if len(list) != 1 {
				t.Fatalf("File() error = %v, want one diagnostic", err)
			}
			got := list[0]
			if got.Code != tt.want.Code || got.Pos != tt.want.Pos {
				t.Errorf("diagnostic = %s at %v, want %s at %v (%v)", got.Code, got.Pos, tt.want.Code, tt.want.Pos, got)
			}
			if tt.want.Message != "" && got.Message != tt.want.Message {
				t.Errorf("Message = %q, want %q", got.Message, tt.want.Message)
			}
			if got.Hint != tt.wantHint {
				t.Errorf("Hint = %q, want %q", got.Hint, tt.wantHint)
			}
		})
	}
}

func TestFieldValues(t *testing.T) {
	type item struct {
		Name  string
		Count int
	}
	tests := []struct {
		name    string
		field   string
		list    any
		want    []string
		wantErr bool
	}{
		{name: "structs", field: "Name", list: []item{{Name: "b"}, {Name: "a"}}, want: []string{"b", "a"}},
		{name: "pointers to structs", field: "Name", list: []*item{{Name: "p"}}, want: []string{"p"}},
		{name: "empty list", field: "Name", list: []item{}, want: []string{}},
		{name: "not a list", field: "Name", list: "x", wantErr: true},
		{name: "not a struct", field: "Name", list: []string{"x"}, wantErr: true},
		{name: "missing field", field: "Missing", list: []item{{}}, wantErr: true},
		{name: "not a string", field: "Count", list: []item{{}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fieldValues(tt.field, tt.list)
			if (err != nil) != tt.wantErr {
				t.Fatalf("fieldValues() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !slices.Equal(got, tt.want) {
				t.Errorf("fieldValues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListFuncs(t *testing.T) {
	tests := []struct {
		name string
		fn   func([]string) []string
		in   []string
		want []string
	}{
		{name: "uniq keeps first occurrences", fn: uniq, in: []string{"b", "a", "b"}, want: []string{"b", "a"}},
		{name: "uniq of empty", fn: uniq, in: nil, want: []string{}},
		{name: "sortAlpha", fn: sortAlpha, in: []string{"b", "c", "a"}, want: []string{"a", "b", "c"}},
		{name: "sortAlpha leaves input alone", fn: sortAlpha, in: []string{"z", "y"}, want: []string{"y", "z"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := slices.Clone(tt.in)
			if got := tt.fn(tt.in); !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			if !slices.Equal(tt.in, in) {
				t.Errorf("input changed to %v", tt.in)
			}
		})
	}
}
