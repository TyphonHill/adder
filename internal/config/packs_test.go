package config

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"github.com/TyphonHill/adder/internal/diag"
)

// fakePacks finds pack.yaml files by pack name.
type fakePacks map[string]string

func (p fakePacks) FS(name string) (fs.FS, bool) {
	content, ok := p[name]
	if !ok {
		return nil, false
	}
	return fstest.MapFS{PackFileName: {Data: []byte(content)}}, true
}

func (fakePacks) Funcs() template.FuncMap { return nil }

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

func TestPacks(t *testing.T) {
	cfg, err := Parse("adder.yaml", []byte(`version: 1
packs:
  drivers:
    pack: store
    with: {dirs: ["drivers/*"], out: drivers}
  mocks:
    pack: store
    with: {dirs: [mocks], out: mocks, tests: true, kind: interface}
`), fakePacks{"store": storePack})
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
		name   string
		config string
		pack   string
		packs  Packs // the store pack is pack when nil
		want   []string
	}{
		{name: "a pack adder was not built with", config: use, packs: fakePacks{}, want: []string{"adder.yaml:4:5 unknown-pack"}},
		{name: "no packs at all", config: use, packs: fakePacks(nil), want: []string{"adder.yaml:4:5 unknown-pack"}},
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
		{name: "generators in pack.yaml", config: use, pack: "version: 1\noptions: {dirs: {type: list}, out: {}}\ngenerators: {}\n", want: []string{"pack store: pack.yaml:3 unknown-field"}},
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
		{name: "a scan named Options", config: "version: 1\nderive:\n  scans:\n    Options: {dirs: [x]}\n", want: []string{"adder.yaml:4:5 reserved-name"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packs := tt.packs
			if packs == nil {
				packs = fakePacks{"store": tt.pack}
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
		wantErr string
	}{
		{name: "a string", option: Option{Type: OptionString}, value: "x", want: "x"},
		{name: "a bool", option: Option{Type: OptionBool}, value: true, want: true},
		{name: "a number for a string", option: Option{Type: OptionString}, value: 5, wantErr: "is 5, want a string"},
		{name: "nothing for a string", option: Option{Type: OptionString}, value: nil, wantErr: "is empty, want a string"},
		{name: "a mapping for a bool", option: Option{Type: OptionBool}, value: map[string]any{}, wantErr: "is a mapping, want true or false"},
		{name: "a list for a string", option: Option{Type: OptionString}, value: []any{"x"}, wantErr: "is a list, want a string"},
		{name: "a number in a list", option: Option{Type: OptionList}, value: []any{"x", 2}, wantErr: "value 1 is 2, want a string"},
		{name: "a value outside the values", option: Option{Type: OptionList, Values: []string{"a"}}, value: []any{"b"}, wantErr: `value 0 "b" is not one of a`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.option.value(tt.value)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("value() error = %v, want %q", err, tt.wantErr)
				}
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
		out  Output
		want string
	}{
		{out: Output{Template: "t.tmpl"}, want: "t.tmpl"},
		{out: Output{Template: "t.tmpl", Pack: "store"}, want: "t.tmpl in pack store"},
		{out: Output{Template: "t.tmpl", Pack: "store", Use: "drivers"}, want: "t.tmpl in pack store for drivers"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.out.Source(); got != tt.want {
				t.Errorf("Source() = %q, want %q", got, tt.want)
			}
		})
	}
}
