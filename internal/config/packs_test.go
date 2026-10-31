package config

import (
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"typhonhill.dev/adder/internal/diag"
)

// fakePacks serves the pack.yaml of each pack by name, and the scanners and functions it maps to what to use
// instead, "" for one that is not deprecated.
type fakePacks struct {
	packs    map[string]string
	scanners map[string]string
	funcs    map[string]string
}

func (p fakePacks) FS(name string) (fs.FS, bool) {
	content, ok := p.packs[name]
	return fstest.MapFS{PackFileName: {Data: []byte(content)}}, ok
}

func (p fakePacks) Funcs() template.FuncMap {
	funcs := template.FuncMap{}
	for name := range p.funcs {
		funcs[name] = strings.ToUpper
	}
	return funcs
}

func (p fakePacks) HasScanner(name string) bool {
	_, ok := p.scanners[name]
	return ok
}

func (p fakePacks) ScannerDeprecation(name string) string { return p.scanners[name] }
func (p fakePacks) DeprecatedFuncs() map[string]string {
	deprecated := map[string]string{}
	for name, instead := range p.funcs {
		if instead != "" {
			deprecated[name] = instead
		}
	}
	return deprecated
}

const storePack = `version: 1
options:
  dirs: {type: list, pattern: "[a-z/*]+"}
  out: {help: Directory for the registry}
  tests: {type: bool}
  kind: {default: struct, values: [struct, interface]}
  ctor: {default: ^New$}
derive:
  scans:
    Drivers:
      dirs: {option: dirs}
      includeTests: {option: tests}
      types: {kind: {option: kind}}
    Ctors: {dirs: [x], funcs: {name: {option: ctor}}}
  outputs:
    - {template: registry.go.tmpl, path: "{{ .Options.out }}/registry.gen.go"}
    - {template: doc.go.tmpl, each: Drivers, path: "{{ .Options.out }}/{{ .Item.Name }}_doc.gen.go"}
`

// TestPacks checks one configuration using a pack twice, field by field.
func TestPacks(t *testing.T) {
	cfg, err := Parse("adder.yaml", []byte(`version: 1
packs:
  drivers:
    pack: store
    with: {dirs: ["drivers/*"], out: drivers}
  mocks:
    pack: store
    with: {dirs: [mocks], out: mocks, tests: true, kind: interface}
`), fakePacks{packs: map[string]string{"store": storePack}})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	tests := []struct {
		use       string
		options   map[string]any
		dirs      []string
		tests     bool
		kind      string
		paths     []string
		outputPos string
	}{
		{
			use:       "drivers",
			options:   map[string]any{"dirs": []string{"drivers/*"}, "out": "drivers", "tests": false, "kind": "struct"},
			dirs:      []string{"drivers/*"},
			kind:      "struct",
			paths:     []string{"drivers/registry.gen.go", "{{ .Options.out }}/{{ .Item.Name }}_doc.gen.go"},
			outputPos: "pack store: pack.yaml:16:7",
		},
		{
			use:       "mocks",
			options:   map[string]any{"dirs": []string{"mocks"}, "out": "mocks", "tests": true, "kind": "interface"},
			dirs:      []string{"mocks"},
			tests:     true,
			kind:      "interface",
			paths:     []string{"mocks/registry.gen.go", "{{ .Options.out }}/{{ .Item.Name }}_doc.gen.go"},
			outputPos: "pack store: pack.yaml:16:7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.use, func(t *testing.T) {
			use := cfg.Packs[tt.use]
			if use.Config == nil || use.FS == nil {
				t.Fatalf("use %q was not loaded", tt.use)
			}
			for name, want := range tt.options {
				got := use.Options[name]
				if list, ok := want.([]string); ok {
					if !slices.Equal(got.([]string), list) {
						t.Errorf("option %s = %v, want %v", name, got, want)
					}
				} else if got != want {
					t.Errorf("option %s = %v, want %v", name, got, want)
				}
			}
			s := use.Config.Derive.Scans["Drivers"]
			if !slices.Equal(s.Dirs, tt.dirs) || s.IncludeTests != tt.tests || s.Types == nil || s.Types.Kind != tt.kind {
				t.Errorf("scan = dirs %v, includeTests %v, types %+v", s.Dirs, s.IncludeTests, s.Types)
			}
			if got := use.Config.Derive.Scans["Ctors"].Funcs.Name.String(); got != "^New$" {
				t.Errorf("Ctors name = %q, want the option's default", got)
			}
			var paths []string
			for _, out := range use.Config.Derive.Outputs {
				paths = append(paths, out.Path)
				if out.Pack != "store" || out.Use != tt.use {
					t.Errorf("output %s comes from pack %q, use %q", out.Path, out.Pack, out.Use)
				}
			}
			if !slices.Equal(paths, tt.paths) {
				t.Errorf("paths = %q, want %q", paths, tt.paths)
			}
			if got := use.Config.Derive.Outputs[0].Pos.String(); got != tt.outputPos {
				t.Errorf("output position = %s, want %s", got, tt.outputPos)
			}
		})
	}
}

func TestPackProblems(t *testing.T) {
	const use = "version: 1\npacks:\n  drivers:\n    pack: store\n    with: {dirs: [\"drivers/*\"], out: drivers}\n"
	tests := []struct {
		name    string
		config  string
		pack    string
		packs   Packs // the store pack is pack when nil
		noPacks bool  // Parse gets nil Packs, as from a build without packs
		want    []string
	}{
		{name: "a pack adder was not built with", config: use, packs: fakePacks{}, want: []string{"adder.yaml:4:5 unknown-pack"}},
		{name: "no packs at all", config: use, noPacks: true, want: []string{"adder.yaml:4:5 unknown-pack"}},
		{name: "a use name that is not lowercase words", config: "version: 1\npacks:\n  Drivers: {pack: store}\n", want: []string{"adder.yaml:3:3 invalid-name"}},
		{name: "a use without a pack", config: "version: 1\npacks:\n  drivers: {with: {}}\n", want: []string{"adder.yaml:3:3 missing-field"}},
		{name: "an unknown field in a use", config: "version: 1\npacks:\n  drivers: {pack: store, options: {}}\n", want: []string{"adder.yaml:3 unknown-field"}},
		{name: "an unknown option", config: "version: 1\npacks:\n  drivers:\n    pack: store\n    with:\n      dirs: [d]\n      out: x\n      extra: x\n", pack: storePack, want: []string{"adder.yaml:8:7 unknown-field"}},
		{name: "an option for a pack without options", config: "version: 1\npacks:\n  drivers: {pack: store, with: {x: y}}\n", pack: "version: 1\n", want: []string{"adder.yaml:3:33 unknown-field"}},
		{name: "a missing option", config: "version: 1\npacks:\n  drivers: {pack: store, with: {out: x}}\n", pack: storePack, want: []string{"adder.yaml:3:3 missing-field"}},
		{name: "a string where a list belongs", config: "version: 1\npacks:\n  drivers: {pack: store, with: {dirs: drivers, out: x}}\n", pack: storePack, want: []string{"adder.yaml:3:33 invalid-value"}},
		{name: "a list value against the pattern", config: "version: 1\npacks:\n  drivers: {pack: store, with: {dirs: [Drivers], out: x}}\n", pack: storePack, want: []string{"adder.yaml:3:33 invalid-value"}},
		{name: "a value not in the values", config: "version: 1\npacks:\n  drivers: {pack: store, with: {dirs: [d], out: x, kind: type}}\n", pack: storePack, want: []string{"adder.yaml:3:52 invalid-value"}},
		{name: "a bool option given a string", config: "version: 1\npacks:\n  drivers: {pack: store, with: {dirs: [d], out: x, tests: yes}}\n", pack: storePack, want: []string{"adder.yaml:3:52 invalid-value"}},
		{name: "a filled-in value the configuration rejects", config: "version: 1\npacks:\n  drivers: {pack: store, with: {dirs: [../d], out: x}}\n", pack: strings.Replace(storePack, `pattern: "[a-z/*]+"`, `help: dirs`, 1), want: []string{"adder.yaml:3:33 unsafe-path"}},
		{name: "a path that leaves the root", config: "version: 1\npacks:\n  drivers: {pack: store, with: {dirs: [d], out: ..}}\n", pack: storePack, want: []string{"adder.yaml:3:3 unsafe-path"}},
		{name: "an empty pack.yaml", config: use, pack: "", want: []string{"pack store: pack.yaml missing-field"}},
		{name: "a pack.yaml that is not YAML", config: use, pack: "version: [1\n", want: []string{"pack store: pack.yaml:1 yaml-syntax"}},
		{name: "a pack.yaml without a version", config: use, pack: "options: {dirs: {type: list}, out: {}}\n", want: []string{"pack store: pack.yaml:1:1 missing-field"}},
		{name: "an unknown field in pack.yaml", config: use, pack: "version: 1\noptions: {dirs: {type: list}, out: {}}\nvars: {}\n", want: []string{"pack store: pack.yaml:3 unknown-field"}},
		{name: "an option pack.yaml does not declare", config: use, pack: "version: 1\noptions: {dirs: {type: list}, out: {}}\nderive:\n  scans:\n    S: {dirs: {option: nope}}\n", want: []string{"pack store: pack.yaml:5:15 invalid-value"}},
		{name: "an option of the wrong type for its field", config: use, pack: "version: 1\noptions: {dirs: {type: list}, out: {}}\nderive:\n  scans:\n    S: {dirs: [x], includeTests: {option: out}}\n", want: []string{"pack store: pack.yaml:5 invalid-value"}},
		{
			name:   "options declared wrongly",
			config: "version: 1\npacks:\n  drivers: {pack: store}\n",
			pack:   "version: 1\noptions:\n  Out: {}\n  n: {type: int}\n  b: {type: bool, pattern: x}\n  d: {default: [x]}\n",
			want:   []string{"pack store: pack.yaml:3:3 invalid-name", "pack store: pack.yaml:5:19 invalid-value", "pack store: pack.yaml:6:7 invalid-value", "pack store: pack.yaml:4:7 invalid-value"},
		},
		{name: "a path using what it cannot", config: use, pack: "version: 1\noptions: {dirs: {type: list}, out: {}}\nderive:\n  outputs:\n    - {template: t.tmpl, path: \"{{ .Vars.x }}.go\"}\n", want: []string{"pack store: pack.yaml:5:26 invalid-value"}},
		{
			name:   "two uses writing one file",
			config: "version: 1\npacks:\n  a: {pack: store, with: {dirs: [a], out: x}}\n  b: {pack: store, with: {dirs: [b], out: x}}\n",
			pack:   storePack,
			want:   []string{"adder.yaml:4:3 duplicate-path"},
		},
		{
			name:   "a use writing the configuration's own output",
			config: "version: 1\nderive:\n  outputs:\n    - {template: t.tmpl, path: x/registry.gen.go}\npacks:\n  a: {pack: store, with: {dirs: [a], out: x}}\n",
			pack:   storePack,
			want:   []string{"adder.yaml:6:3 duplicate-path"},
		},
		{name: "an output rendering a template of a pack adder was not built with", config: "version: 1\nderive:\n  outputs:\n    - {template: t.tmpl, pack: other, path: x.go}\n", want: []string{"adder.yaml:4:26 unknown-pack"}},
		{name: "a generator file rendering a template of a pack adder was not built with", config: "version: 1\ngenerators:\n  g: {files: [{template: t.tmpl, pack: other, path: x.go}]}\n", want: []string{"adder.yaml:3:34 unknown-pack"}},
		{name: "a borrowed pack's bad partial", config: "version: 1\nderive:\n  outputs:\n    - {template: t.tmpl, pack: store, path: x.go}\n", pack: "version: 1\npartials: [\"a/**/b\"]\n", want: []string{"pack store: pack.yaml:2:12 invalid-value"}},
		{name: "a borrowed pack's pack.yaml that does not decode", config: "version: 1\nderive:\n  outputs:\n    - {template: t.tmpl, pack: store, path: x.go}\n", pack: "partials: [[x]]\n", want: []string{"pack store: pack.yaml:1 invalid-value"}},
		{name: "a pack's output naming a pack", config: use, pack: "version: 1\noptions: {dirs: {type: list}, out: {}}\nderive:\n  outputs:\n    - {template: t.tmpl, pack: other, path: x.go}\n", want: []string{"pack store: pack.yaml:5:26 invalid-value"}},
		{name: "a pack's generator file naming a pack", config: use, pack: "version: 1\noptions: {dirs: {type: list}, out: {}}\ngenerators:\n  g: {files: [{template: t.tmpl, pack: other, path: x.go}]}\n", want: []string{"pack store: pack.yaml:4:34 invalid-value"}},
		{name: "a template in a scan's directories", config: "version: 1\nderive:\n  scans:\n    S: {dirs: [\"{{ .Vars.dir }}\"]}\n", want: []string{"adder.yaml:4:16 invalid-value"}},
		{name: "a scan named Options", config: "version: 1\nderive:\n  scans:\n    Options: {dirs: [x]}\n", want: []string{"adder.yaml:4:5 reserved-name"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packs := tt.packs
			switch {
			case tt.noPacks:
				packs = nil
			case packs == nil:
				packs = fakePacks{packs: map[string]string{"store": tt.pack}}
			}
			_, err := Parse("adder.yaml", []byte(tt.config), packs)
			var got []string
			for _, d := range diag.From(err) {
				got = append(got, d.Pos.String()+" "+string(d.Code))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("problems = %q, want %q\n%v", got, tt.want, err)
			}
		})
	}
}

func TestOptionValue(t *testing.T) {
	tests := []struct {
		name    string
		option  Option
		value   any
		want    any
		wantErr bool
	}{
		{name: "a string", option: Option{Type: OptionString}, value: "x", want: "x"},
		{name: "a bool", option: Option{Type: OptionBool}, value: true, want: true},
		{name: "a number for a string", option: Option{Type: OptionString}, value: 5, wantErr: true},
		{name: "nothing for a string", option: Option{Type: OptionString}, value: nil, wantErr: true},
		{name: "a mapping for a bool", option: Option{Type: OptionBool}, value: map[string]any{}, wantErr: true},
		{name: "a list for a string", option: Option{Type: OptionString}, value: []any{"x"}, wantErr: true},
		{name: "a number in a list", option: Option{Type: OptionList}, value: []any{"x", 2}, wantErr: true},
		{name: "a value outside the values", option: Option{Type: OptionList, Values: []string{"a"}}, value: []any{"b"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.option.value(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("value() error = %v, want an error: %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("value() = %v, %v, want %v", got, err, tt.want)
			}
		})
	}
}

func TestSource(t *testing.T) {
	tests := []struct {
		name string
		out  Output
		want string
	}{
		{name: "the configuration's own template", out: Output{Template: "t.tmpl"}, want: "t.tmpl"},
		{name: "a pack's template", out: Output{Template: "t.tmpl", Pack: "store"}, want: "t.tmpl in pack store"},
		{name: "a pack use's output", out: Output{Template: "t.tmpl", Pack: "store", Use: "drivers"}, want: "t.tmpl in pack store for drivers"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.out.Source(); got != tt.want {
				t.Errorf("Source() = %q, want %q", got, tt.want)
			}
		})
	}
}

const generatorPack = `version: 1
options:
  dir: {}
  dirs: {type: list, default: ["drivers/*"]}
derive:
  scans:
    Drivers: {dirs: {option: dirs}, funcs: {name: ^New$}}
generators:
  driver:
    description: Add a driver
    inputs:
      name: {pattern: "[a-z]+", unique: Drivers.Package}
    files:
      - {template: driver.go.tmpl, path: "{{ .Options.dir }}/{{ .Inputs.name }}/driver.go"}
`

func TestPackGenerators(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		pack      string
		want      map[string]string // command: use
		wantTests []string          // the driver generator's tests: want and position
		wantErrs  []string
	}{
		{
			name:   "a use's generators become commands",
			config: "version: 1\npacks:\n  drivers: {pack: store, with: {dir: drivers}}\n",
			want:   map[string]string{"driver": "drivers"},
		},
		{
			name:   "renamed generators",
			config: "version: 1\npacks:\n  a: {pack: store, with: {dir: a}, rename: {driver: a-driver}}\n  b: {pack: store, with: {dir: b}}\n",
			want:   map[string]string{"a-driver": "a", "driver": "b"},
		},
		{
			name:      "tests under the use",
			config:    "version: 1\npacks:\n  drivers:\n    pack: store\n    with: {dir: drivers}\n    tests: {driver: [{name: plain, inputs: {name: example}, want: ./testdata/plain}]}\n",
			want:      map[string]string{"driver": "drivers"},
			wantTests: []string{"testdata/plain adder.yaml:6:22"},
		},
		{
			name:     "a name the configuration's own generator has",
			config:   "version: 1\ngenerators:\n  driver: {files: [{template: t.tmpl, path: x.go}]}\npacks:\n  drivers: {pack: store, with: {dir: drivers}}\n",
			wantErrs: []string{"adder.yaml:5:3 duplicate-name"},
		},
		{
			name:     "two uses with one generator name",
			config:   "version: 1\npacks:\n  a: {pack: store, with: {dir: a}}\n  b: {pack: store, with: {dir: b}}\n",
			wantErrs: []string{"adder.yaml:4:3 duplicate-name"},
		},
		{
			name:     "renaming a generator the pack does not have",
			config:   "version: 1\npacks:\n  drivers: {pack: store, with: {dir: d}, rename: {nope: x}}\n",
			wantErrs: []string{"adder.yaml:3:51 invalid-value"},
		},
		{
			name:     "a new name that is not lowercase words",
			config:   "version: 1\npacks:\n  drivers: {pack: store, with: {dir: d}, rename: {driver: Driver}}\n",
			wantErrs: []string{"adder.yaml:3:51 invalid-name"},
		},
		{
			name:     "tests for a generator the use does not have",
			config:   "version: 1\npacks:\n  drivers: {pack: store, with: {dir: d}, tests: {nope: []}}\n",
			wantErrs: []string{"adder.yaml:3:50 invalid-value"},
		},
		{
			name:     "a test with an undeclared input",
			config:   "version: 1\npacks:\n  drivers:\n    pack: store\n    with: {dir: d}\n    tests: {driver: [{name: plain, inputs: {nam: x}, want: testdata/plain}]}\n",
			wantErrs: []string{"adder.yaml:6:45 invalid-value"},
		},
		{
			name:     "tests in pack.yaml",
			config:   "version: 1\npacks:\n  drivers: {pack: store, with: {dir: d}}\n",
			pack:     generatorPack + "    tests: [{name: plain, want: testdata/plain}]\n",
			wantErrs: []string{"pack store: pack.yaml:15:5 invalid-value"},
		},
		{
			name:     "unique on a scan the pack does not have",
			config:   "version: 1\npacks:\n  drivers: {pack: store, with: {dir: d}}\n",
			pack:     strings.Replace(generatorPack, "unique: Drivers.Package", "unique: Other.Package", 1),
			wantErrs: []string{"pack store: pack.yaml:12:33 invalid-value"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pack := tt.pack
			if pack == "" {
				pack = generatorPack
			}
			cfg, err := Parse("adder.yaml", []byte(tt.config), fakePacks{packs: map[string]string{"store": pack}})
			var errs []string
			for _, d := range diag.From(err) {
				errs = append(errs, d.Pos.String()+" "+string(d.Code))
			}
			if !slices.Equal(errs, tt.wantErrs) {
				t.Fatalf("problems = %q, want %q\n%v", errs, tt.wantErrs, err)
			}
			if err != nil {
				return
			}
			got := make(map[string]string)
			for name, gen := range cfg.Generators {
				got[name] = gen.Use
				if gen.Pack != "store" || gen.Pos.Pack != "store" {
					t.Errorf("generator %q comes from pack %q at %s", name, gen.Pack, gen.Pos)
				}
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("generators = %v, want %v", got, tt.want)
			}
			var tests []string
			for _, test := range cfg.Generators["driver"].Tests {
				tests = append(tests, test.Want+" "+test.Pos.String())
			}
			if !slices.Equal(tests, tt.wantTests) {
				t.Errorf("tests = %q, want %q", tests, tt.wantTests)
			}
		})
	}
}

// TestPackTemplates checks the templates a configuration borrows from a pack. It has a single case, so it is not a
// table.
func TestPackTemplates(t *testing.T) {
	cfg, err := Parse("adder.yaml", []byte(`version: 1
derive:
  scans:
    S: {dirs: [x]}
  outputs:
    - {template: each.tmpl, each: S, path: "a/{{ .Item.Name }}.go"}
    - {template: each.tmpl, pack: store, each: S, path: "b/{{ .Item.Name }}.go"}
generators:
  g: {files: [{template: g.tmpl, pack: store, path: x.go}]}
`), fakePacks{packs: map[string]string{"store": "version: 1\noptions: {dir: {}}\npartials: [./_*.tmpl]\n"}})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	pt, ok := cfg.PackTemplates["store"]
	if !ok || pt.FS == nil || len(pt.Partials) != 1 || pt.Partials[0].Pattern != "_*.tmpl" || pt.Partials[0].Pos.String() != "pack store: pack.yaml:3:12" {
		t.Errorf("PackTemplates = %+v", cfg.PackTemplates)
	}
	if got := cfg.Derive.Outputs[1].Source(); got != "each.tmpl in pack store" {
		t.Errorf("Source() = %q", got)
	}
}
