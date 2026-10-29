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
			set, err := NewSet(templates(tt.template), nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := set.File("t.tmpl", tt.outPath, data)
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
	data := map[string]any{"Names": []string{"a"}, "Module": "example.com/x", "Vars": map[string]string{"License": "MIT"}, "Empty": map[string]string{}}
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
			wantHint: "available fields: Empty, Module, Names, Vars",
		},
		{
			name:     "missing key in a nested map",
			fsys:     templates("{{ .Vars.Licence }}"),
			outPath:  "x.txt",
			want:     diag.Diagnostic{Code: diag.TemplateExec, Pos: diag.Pos{File: "t.tmpl", Line: 1, Column: 8}},
			wantHint: "available fields: License",
		},
		{
			name:     "missing key in an empty map",
			fsys:     templates("{{ .Empty.X }}"),
			outPath:  "x.txt",
			want:     diag.Diagnostic{Code: diag.TemplateExec, Pos: diag.Pos{File: "t.tmpl", Line: 1, Column: 9}},
			wantHint: "no fields are available here",
		},
		{
			name:     "missing key from the root",
			fsys:     templates(`{{ range $k, $v := .Vars }}{{ $.Vars.Nope }}{{ end }}`),
			outPath:  "x.txt",
			want:     diag.Diagnostic{Code: diag.TemplateExec, Pos: diag.Pos{File: "t.tmpl", Line: 1, Column: 31}},
			wantHint: "available fields: License",
		},
		{
			name:     "undefined template without partials",
			fsys:     templates(`{{ template "license" }}`),
			outPath:  "x.txt",
			want:     diag.Diagnostic{Code: diag.TemplateExec, Pos: diag.Pos{File: "t.tmpl", Line: 1, Column: 12}},
			wantHint: "no partials are defined; list templates with {{ define }} blocks under partials in adder.yaml",
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
			set, err := NewSet(tt.fsys, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = set.File("t.tmpl", tt.outPath, data)
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

func TestMapKeys(t *testing.T) {
	type names map[string]string
	data := map[string]any{
		"Vars":  map[string]string{"B": "2", "A": "1"},
		"Named": names{"X": "x"},
		"Items": []string{"a"},
		"Ints":  map[int]string{1: "a"},
	}
	tests := []struct {
		name   string
		ref    string
		want   []string
		wantOK bool
	}{
		{name: "root", ref: "<.Nope>", want: []string{"Ints", "Items", "Named", "Vars"}, wantOK: true},
		{name: "nested", ref: "<.Vars.Nope>", want: []string{"A", "B"}, wantOK: true},
		{name: "from the root variable", ref: "<$.Vars.Nope>", want: []string{"A", "B"}, wantOK: true},
		{name: "named map type", ref: "<.Named.Nope>", want: []string{"X"}, wantOK: true},
		{name: "not a field reference", ref: `<index .Vars "Nope">`},
		{name: "through a missing key", ref: "<.Missing.Nope>"},
		{name: "through a list", ref: "<.Items.Nope>"},
		{name: "map without string keys", ref: "<.Ints.Nope>"},
		{name: "deeper than a string", ref: "<.Vars.A.Nope>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := mapKeys(data, tt.ref)
			if ok != tt.wantOK || !slices.Equal(got, tt.want) {
				t.Errorf("mapKeys(%q) = %v, %v, want %v, %v", tt.ref, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestSetPartials(t *testing.T) {
	fsys := fstest.MapFS{
		"_license.tmpl": {Data: []byte("{{ define \"license\" }}// License: {{ .Vars.License }}{{ end }}\r\nignored text\n")},
		"_names.tmpl":   {Data: []byte(`{{ define "names" }}{{ range . }}{{ . }} {{ end }}{{ end }}{{ define "bare" }}bare{{ end }}`)},
		"main.tmpl":     {Data: []byte("{{ template \"license\" . }}\n{{ template \"names\" .Names }}")},
		"override.tmpl": {Data: []byte(`{{ define "bare" }}mine{{ end }}{{ template "bare" }}`)},
		"after.tmpl":    {Data: []byte(`{{ template "bare" }}`)},
		"partial.tmpl":  {Data: []byte(`{{ template "names" .Names }}`)},
	}
	data := map[string]any{"Names": []string{"a", "b"}, "Vars": map[string]string{"License": "MIT"}}
	set, err := NewSet(fsys, []string{"_license.tmpl", "_names.tmpl"})
	if err != nil {
		t.Fatalf("NewSet() error = %v", err)
	}

	tests := []struct {
		name     string
		template string
		want     string
	}{
		{name: "partials see the data they are given", template: "main.tmpl", want: "// License: MIT\na b "},
		{name: "a template may redefine a partial", template: "override.tmpl", want: "mine"},
		{name: "a redefinition does not leak into later templates", template: "after.tmpl", want: "bare"},
		{name: "partials render the same through any template", template: "partial.tmpl", want: "a b "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := set.File(tt.template, "x.txt", data)
			if err != nil {
				t.Fatalf("File() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("File() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetPartialErrors(t *testing.T) {
	fsys := fstest.MapFS{
		"a.tmpl":       {Data: []byte(`{{ define "license" }}A{{ end }}{{ define "a" }}{{ .Missing }}{{ end }}`)},
		"b.tmpl":       {Data: []byte(`{{ define "license" }}B{{ end }}`)},
		"broken.tmpl":  {Data: []byte("{{ define \"x\" }}\n{{ .Names ")},
		"uses-a.tmpl":  {Data: []byte(`{{ template "a" . }}`)},
		"uses-no.tmpl": {Data: []byte(`{{ template "nope" }}`)},
	}
	tests := []struct {
		name      string
		partials  []string
		template  string // when set, rendered after NewSet succeeds
		wantCodes []diag.Code
		wantPos   diag.Pos // of the first problem
		wantHint  string
	}{
		{name: "missing partial", partials: []string{"missing.tmpl"}, wantCodes: []diag.Code{diag.IO}, wantPos: diag.Pos{File: "missing.tmpl"}},
		{name: "partial syntax", partials: []string{"broken.tmpl"}, wantCodes: []diag.Code{diag.TemplateSyntax}, wantPos: diag.Pos{File: "broken.tmpl", Line: 2}},
		{name: "defined twice", partials: []string{"a.tmpl", "b.tmpl"}, wantCodes: []diag.Code{diag.TemplateSyntax}, wantPos: diag.Pos{File: "b.tmpl"}},
		{
			name:      "reports every problem",
			partials:  []string{"missing.tmpl", "broken.tmpl"},
			wantCodes: []diag.Code{diag.IO, diag.TemplateSyntax},
			wantPos:   diag.Pos{File: "missing.tmpl"},
		},
		{
			name:      "errors inside a partial point at the partial",
			partials:  []string{"a.tmpl"},
			template:  "uses-a.tmpl",
			wantCodes: []diag.Code{diag.TemplateExec},
			wantPos:   diag.Pos{File: "a.tmpl", Line: 1, Column: 51},
			wantHint:  "no fields are available here",
		},
		{
			name:      "undefined template lists the partials",
			partials:  []string{"a.tmpl"},
			template:  "uses-no.tmpl",
			wantCodes: []diag.Code{diag.TemplateExec},
			wantPos:   diag.Pos{File: "uses-no.tmpl", Line: 1, Column: 12},
			wantHint:  "defined templates: a, license",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, err := NewSet(fsys, tt.partials)
			if err == nil && tt.template != "" {
				_, err = set.File(tt.template, "x.txt", map[string]any{})
			}
			list := diag.From(err)
			var codes []diag.Code
			for _, d := range list {
				codes = append(codes, d.Code)
			}
			if !slices.Equal(codes, tt.wantCodes) {
				t.Fatalf("problems = %v, want %v\n%v", codes, tt.wantCodes, err)
			}
			if list[0].Pos != tt.wantPos {
				t.Errorf("Pos = %v, want %v (%v)", list[0].Pos, tt.wantPos, list[0])
			}
			if list[0].Hint != tt.wantHint {
				t.Errorf("Hint = %q, want %q", list[0].Hint, tt.wantHint)
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
