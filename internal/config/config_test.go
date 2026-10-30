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
		{
			name:         "paths on outputs with each are kept as written",
			input:        "version: 1\nderive:\n  scans:\n    Drivers: {dirs: [a]}\n  outputs:\n    - {template: ./t.go.tmpl, path: \"./{{ .Item.Dir }}/../x.go\", each: Drivers}\n",
			wantDirs:     []string{"a"},
			wantTemplate: "t.go.tmpl",
			wantPath:     "./{{ .Item.Dir }}/../x.go",
			wantScanPos:  diag.Pos{File: "adder.yaml", Line: 4, Column: 5},
			wantOutPos:   diag.Pos{File: "adder.yaml", Line: 6, Column: 7},
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

const generatorConfig = `version: 1
derive:
  scans:
    Drivers: {dirs: [drivers/*]}
generators:
  driver:
    description: Add a storage driver
    inputs:
      name: {pattern: "[a-z]+", unique: Drivers.Package}
      summary: {default: A driver.}
      cache: {type: bool}
    files:
      - template: ./templates/driver.go.tmpl
        path: drivers/{{ .Inputs.name }}/{{ .Inputs.name }}.go
      - template: templates/cache.go.tmpl
        path: drivers/{{ .Inputs.name }}/cache.go
        when: cache
        ifExists: skip
`

func TestParseGenerators(t *testing.T) {
	cfg, err := Parse("adder.yaml", []byte(generatorConfig))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	gen := cfg.Generators["driver"]
	if gen.Pos != (diag.Pos{File: "adder.yaml", Line: 6, Column: 3}) {
		t.Errorf("generator Pos = %v", gen.Pos)
	}
	if gen.Derive != nil {
		t.Errorf("Derive = %v, want unset", *gen.Derive)
	}

	inputs := []struct {
		name         string
		wantType     string
		wantRequired bool
		match        []string
		noMatch      []string
	}{
		{name: "name", wantType: InputString, wantRequired: true, match: []string{"gcs"}, noMatch: []string{"gcs2", "GCS", ""}},
		{name: "summary", wantType: InputString, match: []string{"", "Anything at all"}},
		{name: "cache", wantType: InputBool},
	}
	for _, tt := range inputs {
		t.Run("input "+tt.name, func(t *testing.T) {
			in := gen.Inputs[tt.name]
			if in.Type != tt.wantType || in.Required() != tt.wantRequired {
				t.Errorf("type %q, required %v, want %q, %v", in.Type, in.Required(), tt.wantType, tt.wantRequired)
			}
			if in.Pos.Line == 0 {
				t.Error("input has no position")
			}
			for _, v := range tt.match {
				if !in.MatchesPattern(v) {
					t.Errorf("MatchesPattern(%q) = false", v)
				}
			}
			for _, v := range tt.noMatch {
				if in.MatchesPattern(v) {
					t.Errorf("MatchesPattern(%q) = true", v)
				}
			}
		})
	}

	files := []struct {
		wantTemplate string
		wantIfExists string
		wantPos      diag.Pos
	}{
		{wantTemplate: "templates/driver.go.tmpl", wantIfExists: IfExistsFail, wantPos: diag.Pos{File: "adder.yaml", Line: 13, Column: 9}},
		{wantTemplate: "templates/cache.go.tmpl", wantIfExists: IfExistsSkip, wantPos: diag.Pos{File: "adder.yaml", Line: 15, Column: 9}},
	}
	for i, tt := range files {
		f := gen.Files[i]
		if f.Template != tt.wantTemplate || f.IfExists != tt.wantIfExists || f.Pos != tt.wantPos {
			t.Errorf("file %d = %q, %q at %v, want %q, %q at %v", i, f.Template, f.IfExists, f.Pos, tt.wantTemplate, tt.wantIfExists, tt.wantPos)
		}
	}
}

func TestFlag(t *testing.T) {
	tests := []struct{ input, want string }{
		{"name", "name"},
		{"packageName", "package-name"},
		{"userID", "user-id"},
		{"http2Server", "http2-server"},
	}
	for _, tt := range tests {
		if got := Flag(tt.input); got != tt.want {
			t.Errorf("Flag(%q) = %q, want %q", tt.input, got, tt.want)
		}
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
		{
			name:     "build tag with a space",
			input:    "version: 1\nderive:\n  scans:\n    S:\n      dirs: [a]\n      buildTags: [ok, \"two words\"]",
			want:     []want{{diag.InvalidValue, 6, 23}},
			wantHint: "list tags as //go:build lines name them, such as integration",
		},
		{name: "test without a name or want", input: "version: 1\ngenerators:\n  g:\n    inputs: {name: {}}\n    files: [{template: t.tmpl, path: x}]\n    tests:\n      - inputs: {name: a}\n", want: []want{{diag.MissingField, 7, 9}, {diag.MissingField, 7, 9}}},
		{name: "test name not kebab case", input: "version: 1\ngenerators:\n  g:\n    inputs: {name: {}}\n    files: [{template: t.tmpl, path: x}]\n    tests:\n      - {name: With_Cache, want: w}\n", want: []want{{diag.InvalidName, 7, 10}}},
		{name: "two tests with one name", input: "version: 1\ngenerators:\n  g:\n    inputs: {name: {}}\n    files: [{template: t.tmpl, path: x}]\n    tests:\n      - {name: a, want: w}\n      - {name: a, want: v}\n", want: []want{{diag.InvalidName, 8, 10}}},
		{name: "test want escapes the root", input: "version: 1\ngenerators:\n  g:\n    inputs: {name: {}}\n    files: [{template: t.tmpl, path: x}]\n    tests:\n      - {name: a, want: ../w}\n", want: []want{{diag.UnsafePath, 7, 19}}},
		{
			name:     "test with an unknown input",
			input:    "version: 1\ngenerators:\n  g:\n    inputs: {name: {}}\n    files: [{template: t.tmpl, path: x}]\n    tests:\n      - {name: a, want: w, inputs: {nam: x}}\n",
			want:     []want{{diag.InvalidValue, 7, 37}},
			wantHint: "inputs are keyed by input name, not flag, such as packageName",
		},
		{name: "test want is the configuration directory", input: "version: 1\ngenerators:\n  g:\n    inputs: {name: {}}\n    files: [{template: t.tmpl, path: x}]\n    tests:\n      - {name: a, want: ./}\n", want: []want{{diag.InvalidValue, 7, 19}}},
		{name: "unknown test field", input: "version: 1\ngenerators:\n  g:\n    inputs: {name: {}}\n    files: [{template: t.tmpl, path: x}]\n    tests:\n      - {name: a, want: w, input: {}}\n", want: []want{{diag.UnknownField, 7, 0}}},
		{name: "values with an unknown kind", input: "version: 1\nderive:\n  scans:\n    S:\n      dirs: [a]\n      values: {kind: constant}\n", want: []want{{diag.InvalidValue, 6, 16}}},
		{name: "values with an invalid directive", input: "version: 1\nderive:\n  scans:\n    S:\n      dirs: [a]\n      values: {directive: Bad}\n", want: []want{{diag.InvalidValue, 6, 16}}},
		{
			name:        "values and types together",
			input:       "version: 1\nderive:\n  scans:\n    S:\n      dirs: [a]\n      types: {}\n      values: {}\n",
			want:        []want{{diag.InvalidValue, 7, 7}},
			wantMessage: `scan "S": has types and values; use only one`,
		},
		{name: "scan name reserved for items", input: "version: 1\nderive:\n  scans:\n    Item: {dirs: [a]}", want: []want{{diag.ReservedName, 4, 5}}},
		{
			name:     "templated path without each",
			input:    "version: 1\nderive:\n  scans:\n    S: {dirs: [a]}\n  outputs:\n    - template: t.tmpl\n      path: \"{{ .Vars.Dir }}/x.go\"\n",
			want:     []want{{diag.InvalidValue, 7, 7}},
			wantHint: "only outputs with each, which render one file per scan item, have templated paths",
		},
		{name: "each names no scan", input: "version: 1\nderive:\n  scans:\n    S: {dirs: [a]}\n  outputs:\n    - template: t.tmpl\n      path: \"{{ .Item.Name }}.go\"\n      each: Nope\n", want: []want{{diag.InvalidValue, 8, 7}}},
		{
			name:  "each with a shared template",
			input: "version: 1\nderive:\n  scans:\n    S: {dirs: [a]}\n  outputs:\n    - template: t.tmpl\n      path: a.go\n    - template: t.tmpl\n      path: \"{{ .Item.Name }}.go\"\n      each: S\n",
			want:  []want{{diag.InvalidValue, 8, 7}},
		},
		{
			name:     "each path without a header",
			input:    "version: 1\nderive:\n  scans:\n    S: {dirs: [a]}\n  outputs:\n    - template: t.tmpl\n      path: \"{{ .Item.Name }}.json\"\n      each: S\n",
			want:     []want{{diag.InvalidValue, 7, 7}},
			wantHint: "stale files are found by adder's header, so the path needs a literal extension with comments, such as .go, .ts, .yaml, or .sql",
		},
		{name: "each path ending in an action", input: "version: 1\nderive:\n  scans:\n    S: {dirs: [a]}\n  outputs:\n    - template: t.tmpl\n      path: \"x.{{ .Item.Ext }}\"\n      each: S\n", want: []want{{diag.InvalidValue, 7, 7}}},
		{name: "each without a path", input: "version: 1\nderive:\n  scans:\n    S: {dirs: [a]}\n  outputs:\n    - template: t.tmpl\n      each: S\n", want: []want{{diag.MissingField, 6, 7}}},
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
		{name: "generator name not kebab-case", input: "version: 1\ngenerators:\n  addDriver: {files: [{template: t, path: p}]}", want: []want{{diag.InvalidName, 3, 3}}, wantHint: "generator names are commands, such as adder new http-handler"},
		{name: "generator without files", input: "version: 1\ngenerators:\n  g: {description: x}", want: []want{{diag.MissingField, 3, 3}}},
		{name: "input name not lowerCamel", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    inputs: {PackageName: {}}\n    files: [{template: t, path: p}]", want: []want{{diag.InvalidName, 5, 14}}, wantHint: "input names become template fields and flags: packageName is .Inputs.packageName and --package-name"},
		{name: "input uses a reserved flag", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    inputs: {dryRun: {type: bool}}\n    files: [{template: t, path: p}]", want: []want{{diag.ReservedName, 5, 14}}},
		{name: "inputs share a flag", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    inputs: {userID: {}, userId: {}}\n    files: [{template: t, path: p}]", want: []want{{diag.InvalidName, 5, 26}}},
		{name: "unknown input type", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    inputs: {n: {type: int}}\n    files: [{template: t, path: p}]", want: []want{{diag.InvalidValue, 5, 18}}, wantHint: "type is string or bool"},
		{name: "bool input with string settings", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    inputs: {c: {type: bool, default: \"true\", pattern: x}}\n    files: [{template: t, path: p}]", want: []want{{diag.InvalidValue, 5, 30}, {diag.InvalidValue, 5, 47}}, wantHint: "bool inputs are false unless their flag is given"},
		{name: "values and valuesFrom", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    inputs: {n: {values: [a], valuesFrom: S.Dir}}\n    files: [{template: t, path: p}]", want: []want{{diag.InvalidValue, 5, 31}}},
		{name: "default fails the pattern", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    inputs: {n: {default: X, pattern: \"[a-z]+\"}}\n    files: [{template: t, path: p}]", want: []want{{diag.InvalidValue, 5, 18}}},
		{name: "default not among values", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    inputs: {n: {default: c, values: [a, b]}}\n    files: [{template: t, path: p}]", want: []want{{diag.InvalidValue, 5, 18}}},
		{name: "malformed scan reference", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    inputs: {n: {unique: Drivers}}\n    files: [{template: t, path: p}]", want: []want{{diag.InvalidValue, 5, 18}}, wantHint: "write Scan.Field, such as Drivers.Package"},
		{name: "unknown scan reference", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    inputs: {n: {valuesFrom: Nope.Dir}}\n    files: [{template: t, path: p}]", want: []want{{diag.InvalidValue, 5, 18}}},
		{name: "file without template or path", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    files: [{when: x}]", want: []want{{diag.MissingField, 5, 13}, {diag.MissingField, 5, 13}, {diag.InvalidValue, 5, 14}}},
		{name: "file template escapes root", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    files: [{template: ../t, path: p}]", want: []want{{diag.UnsafePath, 5, 14}}},
		{name: "when and unless", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    inputs: {a: {type: bool}}\n    files: [{template: t, path: p, when: a, unless: a}]", want: []want{{diag.InvalidValue, 6, 45}}},
		{name: "when names a string input", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    inputs: {a: {}}\n    files: [{template: t, path: p, when: a}]", want: []want{{diag.InvalidValue, 6, 36}}, wantHint: "when and unless take an input name, such as when: cache"},
		{name: "unknown ifExists", input: "version: 1\nderive: {scans: {S: {dirs: [a]}}}\ngenerators:\n  g:\n    files: [{template: t, path: p, ifExists: overwrite}]", want: []want{{diag.InvalidValue, 5, 36}}, wantHint: "ifExists is fail or skip; adder never overwrites a file people own"},
		{name: "unknown generator field", input: "version: 1\ngenerators:\n  g: {fils: []}", want: []want{{diag.UnknownField, 3, 0}}, wantHint: "known fields: description, inputs, files, message, derive, tests"},
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

func TestFind(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"a/b/c", "nested/adder.yaml"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"adder.yaml", "a/b/adder.yaml"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(file)), []byte("version: 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name string
		cwd  string // relative to root
		dir  string
		want string // relative to cwd, or absolute when dir is
	}{
		{name: "in the directory", cwd: ".", dir: ".", want: "adder.yaml"},
		{name: "in a parent", cwd: "a", dir: ".", want: filepath.Join("..", "adder.yaml")},
		{name: "the nearest one wins", cwd: "a/b/c", dir: ".", want: filepath.Join("..", "adder.yaml")},
		{name: "a relative directory", cwd: ".", dir: filepath.Join("a", "b", "c"), want: filepath.Join("a", "b", "adder.yaml")},
		{name: "directories with the name are skipped", cwd: "nested", dir: ".", want: filepath.Join("..", "adder.yaml")},
		{name: "an absolute directory", cwd: ".", dir: filepath.Join(root, "a"), want: filepath.Join(root, "adder.yaml")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(filepath.Join(root, filepath.FromSlash(tt.cwd)))
			got, err := Find(tt.dir)
			if err != nil || got != tt.want {
				t.Errorf("Find(%q) = %q, %v, want %q", tt.dir, got, err, tt.want)
			}
		})
	}

	t.Run("not found", func(t *testing.T) {
		_, err := Find(t.TempDir())
		list := diag.From(err)
		if len(list) != 1 || list[0].Code != diag.IO || !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Find() error = %v, want an io problem wrapping fs.ErrNotExist", err)
		}
	})
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
