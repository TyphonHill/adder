package config

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/TyphonHill/adder/internal/diag"
)

const validConfig = `version: 1
derive:
  scans:
    Drivers:
      dirs: ["./drivers/*"]
      funcs:
        exported: true
        name: ^New$
  outputs:
    - template: ./templates/drivers.go.tmpl
      path: drivers/drivers.gen.go
`

func TestParse(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantDirs     []string
		wantTemplate string
		wantPath     string
		wantScanPos  diag.Pos
		wantOutPos   diag.Pos
		wantMatch    []string
		wantNoMatch  []string
	}{
		{
			name:         "cleans paths and records positions",
			input:        validConfig,
			wantDirs:     []string{"drivers/*"},
			wantTemplate: "templates/drivers.go.tmpl",
			wantPath:     "drivers/drivers.gen.go",
			wantScanPos:  diag.Pos{File: "adder.yaml", Line: 4, Column: 5},
			wantOutPos:   diag.Pos{File: "adder.yaml", Line: 10, Column: 7},
			wantMatch:    []string{"New"},
			wantNoMatch:  []string{"NewOptions", "Renew"},
		},
		{
			name:         "double star and exclude patterns are cleaned",
			input:        "version: 1\nderive:\n  scans:\n    Drivers: {dirs: [./internal/**/drivers], exclude: [./internal/legacy/**]}\n  outputs: [{template: t.tmpl, path: out.go}]\n",
			wantDirs:     []string{"internal/**/drivers"},
			wantTemplate: "t.tmpl",
			wantPath:     "out.go",
			wantScanPos:  diag.Pos{File: "adder.yaml", Line: 4, Column: 5},
			wantOutPos:   diag.Pos{File: "adder.yaml", Line: 5, Column: 13},
			wantMatch:    []string{"Anything"},
		},
		{
			name:         "type scans",
			input:        "version: 1\nderive:\n  scans:\n    Drivers: {dirs: [drivers], types: {kind: struct, directive: register}}\n  outputs: [{template: t.tmpl, path: out.go}]\n",
			wantDirs:     []string{"drivers"},
			wantTemplate: "t.tmpl",
			wantPath:     "out.go",
			wantScanPos:  diag.Pos{File: "adder.yaml", Line: 4, Column: 5},
			wantOutPos:   diag.Pos{File: "adder.yaml", Line: 5, Column: 13},
			wantMatch:    []string{"Anything"},
		},
		{
			name:         "flow style",
			input:        "version: 1\nderive:\n  scans: {Drivers: {dirs: [drivers]}}\n  outputs: [{template: t.tmpl, path: out.go}]\n",
			wantDirs:     []string{"drivers"},
			wantTemplate: "t.tmpl",
			wantPath:     "out.go",
			wantScanPos:  diag.Pos{File: "adder.yaml", Line: 3, Column: 11},
			wantOutPos:   diag.Pos{File: "adder.yaml", Line: 4, Column: 13},
			wantMatch:    []string{"Anything"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse("adder.yaml", []byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			scan := cfg.Derive.Scans["Drivers"]
			if !slices.Equal(scan.Dirs, tt.wantDirs) {
				t.Errorf("Dirs = %v, want %v", scan.Dirs, tt.wantDirs)
			}
			if scan.Pos != tt.wantScanPos {
				t.Errorf("scan Pos = %v, want %v", scan.Pos, tt.wantScanPos)
			}
			for _, name := range tt.wantMatch {
				if scan.Funcs.Name.Regexp != nil && !scan.Funcs.Name.MatchString(name) {
					t.Errorf("pattern %q does not match %q", scan.Funcs.Name, name)
				}
			}
			for _, name := range tt.wantNoMatch {
				if scan.Funcs.Name.MatchString(name) {
					t.Errorf("pattern %q matches %q", scan.Funcs.Name, name)
				}
			}

			out := cfg.Derive.Outputs[0]
			if out.Template != tt.wantTemplate || out.Path != tt.wantPath {
				t.Errorf("output = %q -> %q, want %q -> %q", out.Template, out.Path, tt.wantTemplate, tt.wantPath)
			}
			if out.Pos != tt.wantOutPos {
				t.Errorf("output Pos = %v, want %v", out.Pos, tt.wantOutPos)
			}
		})
	}
}

func TestParseShared(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantVars     map[string]string
		wantPartials []Partial
	}{
		{name: "neither", input: "version: 1\n"},
		{
			name:     "vars are strings",
			input:    "version: 1\nvars:\n  License: Apache-2.0\n  Year: 2026\n  Empty:\n",
			wantVars: map[string]string{"License": "Apache-2.0", "Year": "2026", "Empty": ""},
		},
		{
			name:  "partials are cleaned and positioned",
			input: "version: 1\npartials:\n  - ./templates/_*.tmpl\n  - partials/license.tmpl\n",
			wantPartials: []Partial{
				{Pattern: "templates/_*.tmpl", Pos: diag.Pos{File: "adder.yaml", Line: 3, Column: 5}},
				{Pattern: "partials/license.tmpl", Pos: diag.Pos{File: "adder.yaml", Line: 4, Column: 5}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse("adder.yaml", []byte(tt.input))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if !maps.Equal(cfg.Vars, tt.wantVars) {
				t.Errorf("Vars = %v, want %v", cfg.Vars, tt.wantVars)
			}
			if !slices.Equal(cfg.Partials, tt.wantPartials) {
				t.Errorf("Partials = %v, want %v", cfg.Partials, tt.wantPartials)
			}
		})
	}
}

// want is the code and position expected for one diagnostic.
type want struct {
	code   diag.Code
	line   int
	column int
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		want        []want
		wantMessage string // when set, the first diagnostic's message
		wantHint    string // when set, the first diagnostic's hint
	}{
		{name: "empty", input: "", want: []want{{diag.MissingField, 0, 0}}},
		{name: "yaml syntax", input: "version: 1\n  bad: [", want: []want{{diag.YAMLSyntax, 2, 0}}},
		{name: "missing version", input: "derive: {}", want: []want{{diag.MissingField, 1, 1}}},
		{name: "unsupported version", input: "version: 2", want: []want{{diag.UnsupportedVersion, 1, 1}}},
		{name: "wrong type", input: "version: one", want: []want{{diag.InvalidValue, 1, 0}}},
		{
			name:        "unknown field",
			input:       "version: 1\nderive:\n  scanz: {}",
			want:        []want{{diag.UnknownField, 3, 0}},
			wantMessage: `unknown field "scanz"`,
			wantHint:    "known fields: scans, outputs",
		},
		{name: "invalid pattern", input: "version: 1\nderive:\n  scans:\n    A:\n      dirs: [a]\n      funcs: {name: \"(\"}", want: []want{{diag.InvalidValue, 6, 21}}},
		{name: "scan name not identifier", input: "version: 1\nderive:\n  scans:\n    my-scan: {dirs: [a]}", want: []want{{diag.InvalidName, 4, 5}}},
		{name: "scan name reserved", input: "version: 1\nderive:\n  scans:\n    Module: {dirs: [a]}", want: []want{{diag.ReservedName, 4, 5}}},
		{name: "scan name reserved for inputs", input: "version: 1\nderive:\n  scans:\n    Inputs: {dirs: [a]}", want: []want{{diag.ReservedName, 4, 5}}},
		{name: "scan name reserved for vars", input: "version: 1\nderive:\n  scans:\n    Vars: {dirs: [a]}", want: []want{{diag.ReservedName, 4, 5}}},
		{
			name:     "var name not identifier",
			input:    "version: 1\nvars:\n  license-name: MIT",
			want:     []want{{diag.InvalidName, 3, 3}},
			wantHint: "templates read vars as .Vars.<name>, such as .Vars.License",
		},
		{name: "var value not a string", input: "version: 1\nvars:\n  Authors: [a, b]", want: []want{{diag.InvalidValue, 3, 0}}},
		{name: "partial empty", input: "version: 1\npartials: ['']", want: []want{{diag.MissingField, 2, 12}}},
		{name: "partial escapes root", input: "version: 1\npartials: [../shared/*.tmpl]", want: []want{{diag.UnsafePath, 2, 12}}},
		{
			name:     "partial with double star",
			input:    "version: 1\npartials: [templates/**/*.tmpl]",
			want:     []want{{diag.InvalidValue, 2, 12}},
			wantHint: "list each directory, such as templates/partials/*.tmpl",
		},
		{name: "partial bad glob", input: "version: 1\npartials: ['templates/[']", want: []want{{diag.InvalidValue, 2, 12}}},
		{name: "partial not a string", input: "version: 1\npartials: [{a: b}]", want: []want{{diag.InvalidValue, 2, 0}}},
		{name: "scan without dirs", input: "version: 1\nderive:\n  scans:\n    A: {}", want: []want{{diag.MissingField, 4, 5}}},
		{name: "dirs escape root", input: "version: 1\nderive:\n  scans:\n    A: {dirs: [../a]}", want: []want{{diag.UnsafePath, 4, 16}}},
		{
			name:     "dirs with backslash",
			input:    "version: 1\nderive:\n  scans:\n    A: {dirs: ['a\\b']}",
			want:     []want{{diag.UnsafePath, 4, 16}},
			wantHint: "paths in adder.yaml are slash-separated on every operating system",
		},
		{name: "dirs bad glob", input: "version: 1\nderive:\n  scans:\n    A: {dirs: ['a/[']}", want: []want{{diag.InvalidValue, 4, 16}}},
		{
			name:     "unknown kind",
			input:    "version: 1\nderive:\n  scans:\n    A: {dirs: [a], funcs: {kind: type}}",
			want:     []want{{diag.InvalidValue, 4, 28}},
			wantHint: "kind is func or method",
		},
		{name: "receiver not a type name", input: "version: 1\nderive:\n  scans:\n    A: {dirs: [a], funcs: {receiver: \"*Worker\"}}", want: []want{{diag.InvalidValue, 4, 28}}},
		{name: "receiver with kind func", input: "version: 1\nderive:\n  scans:\n    A: {dirs: [a], funcs: {kind: func, receiver: Worker}}", want: []want{{diag.InvalidValue, 4, 40}}},
		{
			name:     "double star inside a segment",
			input:    "version: 1\nderive:\n  scans:\n    A: {dirs: [a/**b]}",
			want:     []want{{diag.InvalidValue, 4, 16}},
			wantHint: "write a/**/b, not a/**b",
		},
		{name: "exclude escapes root", input: "version: 1\nderive:\n  scans:\n    A: {dirs: [a], exclude: [../b]}", want: []want{{diag.UnsafePath, 4, 30}}},
		{name: "exclude bad glob", input: "version: 1\nderive:\n  scans:\n    A: {dirs: [a], exclude: ['b/[']}", want: []want{{diag.InvalidValue, 4, 30}}},
		{name: "funcs and types together", input: "version: 1\nderive:\n  scans:\n    A: {dirs: [a], funcs: {}, types: {}}", want: []want{{diag.InvalidValue, 4, 31}}},
		{
			name:     "unknown type kind",
			input:    "version: 1\nderive:\n  scans:\n    A: {dirs: [a], types: {kind: func}}",
			want:     []want{{diag.InvalidValue, 4, 28}},
			wantHint: "kind is struct, interface, or type",
		},
		{
			name:     "invalid function directive",
			input:    "version: 1\nderive:\n  scans:\n    A: {dirs: [a], funcs: {directive: Register}}",
			want:     []want{{diag.InvalidValue, 4, 28}},
			wantHint: "directive names are lowercase letters, digits, and hyphens, starting with a letter",
		},
		{name: "invalid type directive", input: "version: 1\nderive:\n  scans:\n    A: {dirs: [a], types: {directive: \"adder:x\"}}", want: []want{{diag.InvalidValue, 4, 28}}},
		{name: "output without template", input: "version: 1\nderive:\n  outputs: [{path: a.go}]", want: []want{{diag.MissingField, 3, 13}}},
		{name: "output without path", input: "version: 1\nderive:\n  outputs: [{template: a.tmpl}]", want: []want{{diag.MissingField, 3, 13}}},
		{name: "template escapes root", input: "version: 1\nderive:\n  outputs: [{template: ../a.tmpl, path: a.go}]", want: []want{{diag.UnsafePath, 3, 14}}},
		{name: "output escapes root", input: "version: 1\nderive:\n  outputs: [{template: a.tmpl, path: ../a.go}]", want: []want{{diag.UnsafePath, 3, 32}}},
		{name: "output absolute", input: "version: 1\nderive:\n  outputs: [{template: a.tmpl, path: /tmp/a.go}]", want: []want{{diag.UnsafePath, 3, 32}}},
		{name: "duplicate output", input: "version: 1\nderive:\n  outputs: [{template: a.tmpl, path: a.go}, {template: b.tmpl, path: ./a.go}]", want: []want{{diag.DuplicatePath, 3, 64}}},
		{
			name:  "reports every problem",
			input: "derive:\n  scans:\n    A: {}\n  outputs: [{template: a.tmpl}]",
			want:  []want{{diag.MissingField, 1, 1}, {diag.MissingField, 3, 5}, {diag.MissingField, 4, 13}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse("adder.yaml", []byte(tt.input))
			list := diag.From(err)
			if len(list) == 0 {
				t.Fatal("Parse() error = nil")
			}

			var got []want
			for _, d := range list {
				if d.Pos.File != "adder.yaml" {
					t.Errorf("diagnostic %q has file %q, want adder.yaml", d.Message, d.Pos.File)
				}
				got = append(got, want{d.Code, d.Pos.Line, d.Pos.Column})
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("diagnostics = %v, want %v\n%v", got, tt.want, err)
			}
			if tt.wantMessage != "" && list[0].Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", list[0].Message, tt.wantMessage)
			}
			if tt.wantHint != "" && list[0].Hint != tt.wantHint {
				t.Errorf("Hint = %q, want %q", list[0].Hint, tt.wantHint)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		content  *string // nil means the file does not exist
		wantCode diag.Code
		wantIs   error
	}{
		{name: "valid", content: new(validConfig)},
		{name: "missing", wantCode: diag.IO, wantIs: fs.ErrNotExist},
		{name: "invalid", content: new("version: 2"), wantCode: diag.UnsupportedVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, FileName)
			if tt.content != nil {
				if err := os.WriteFile(path, []byte(*tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			cfg, err := Load(path)
			if tt.wantCode != "" {
				list := diag.From(err)
				if len(list) != 1 || list[0].Code != tt.wantCode || list[0].Pos.File != path {
					t.Fatalf("Load() error = %#v, want %s in %s", list, tt.wantCode, path)
				}
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Errorf("Load() error does not wrap %v", tt.wantIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Dir != dir {
				t.Errorf("Dir = %q, want %q", cfg.Dir, dir)
			}
			if got := cfg.Derive.Scans["Drivers"].Pos.File; got != path {
				t.Errorf("positions name %q, want %q", got, path)
			}
		})
	}
}
