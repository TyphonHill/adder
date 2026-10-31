package config

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"typhonhill.dev/adder/internal/diag"
)

// deprecationPacks has the packs it maps to their pack.yaml, and the scanners it maps to what to use instead,
// "" for one that is not deprecated.
type deprecationPacks struct {
	packs    map[string]string
	scanners map[string]string
	funcs    map[string]string // what to use instead of each function, "" for one that is not deprecated
}

func (p deprecationPacks) FS(name string) (fs.FS, bool) {
	content, ok := p.packs[name]
	return fstest.MapFS{PackFileName: {Data: []byte(content)}}, ok
}

func (p deprecationPacks) Funcs() template.FuncMap {
	funcs := template.FuncMap{}
	for name := range p.funcs {
		funcs[name] = strings.ToUpper
	}
	return funcs
}

func (p deprecationPacks) HasScanner(name string) bool {
	_, ok := p.scanners[name]
	return ok
}

func (p deprecationPacks) ScannerDeprecation(name string) string { return p.scanners[name] }
func (p deprecationPacks) DeprecatedFuncs() map[string]string {
	deprecated := map[string]string{}
	for name, instead := range p.funcs {
		if instead != "" {
			deprecated[name] = instead
		}
	}
	return deprecated
}

const registryPack = `version: 1
options:
  dirs: {type: list}
  out: {default: registry, deprecated: use dir}
  dir: {default: registry}
derive:
  scans:
    Entries: {dirs: {option: dirs}, funcs: {name: ^New$}}
`

func TestWarnings(t *testing.T) {
	tests := []struct {
		name     string
		settings []deprecatedSetting
		config   string
		packs    deprecationPacks
		want     []string // each warning, as pos: message
		wantErr  []string // each problem, as pos code, instead of warnings
	}{
		{
			name:   "nothing deprecated",
			config: "version: 1\nderive:\n  scans:\n    D: {dirs: [d], funcs: {name: ^New$}}\n",
		},
		{
			name:     "a setting",
			settings: []deprecatedSetting{{Path: "derive.scans.*.funcs.name", Message: "use funcs.match"}},
			config:   "version: 1\nderive:\n  scans:\n    D: {dirs: [d], funcs: {name: ^New$}}\n    E: {dirs: [e]}\n",
			want:     []string{"adder.yaml:4:28: derive.scans.D.funcs.name is deprecated: use funcs.match"},
		},
		{
			name:     "a value of a setting",
			settings: []deprecatedSetting{{Path: "derive.scans.*.types.kind", Value: "other", Message: "use named"}},
			config:   "version: 1\nderive:\n  scans:\n    A: {dirs: [a], types: {kind: other}}\n    B: {dirs: [b], types: {kind: struct}}\n",
			want:     []string{`adder.yaml:4:34: derive.scans.A.types.kind: "other" is deprecated: use named`},
		},
		{
			name:     "relying on a default",
			settings: []deprecatedSetting{{Path: "generators.*.files.*.ifExists", Default: true, Message: "set ifExists: fail to keep it"}},
			config:   "version: 1\ngenerators:\n  g:\n    files:\n      - {template: t, path: a}\n      - {template: t, path: b, ifExists: skip}\n",
			want:     []string{"adder.yaml:5:9: generators.g.files.0.ifExists is left out, which relies on a default that is deprecated: set ifExists: fail to keep it"},
		},
		{
			name:     "a setting in a pack",
			settings: []deprecatedSetting{{Path: "derive.scans.*.funcs.name", Message: "use funcs.match"}},
			config:   "version: 1\npacks:\n  caches: {pack: registry, with: {dirs: [\"caches/*\"]}}\n",
			packs:    deprecationPacks{packs: map[string]string{"registry": registryPack}},
			want:     []string{"pack registry: pack.yaml:8:45: derive.scans.Entries.funcs.name is deprecated: use funcs.match"},
		},
		{
			name:   "a deprecated option a use gives",
			config: "version: 1\npacks:\n  caches: {pack: registry, with: {dirs: [\"caches/*\"], out: caches}}\n  queues: {pack: registry, with: {dirs: [\"queues/*\"], dir: queues}}\n",
			packs:  deprecationPacks{packs: map[string]string{"registry": registryPack}},
			want:   []string{`adder.yaml:3:55: pack use "caches": option "out" is deprecated: use dir`},
		},
		{
			name:   "a deprecated function in a pack's output path",
			config: "version: 1\npacks:\n  caches: {pack: p, with: {out: caches}}\n",
			packs:  deprecationPacks{packs: map[string]string{"p": "version: 1\noptions: {out: {}}\nderive:\n  outputs:\n    - {template: t.tmpl, path: \"{{ shout .Options.out }}/x.go\"}\n"}, funcs: map[string]string{"shout": "use upper"}},
			want:   []string{`pack p: pack.yaml:5:26: function "shout" is deprecated: use upper`},
		},
		{
			name:   "a deprecated scanner",
			config: "version: 1\nderive:\n  scans:\n    A: {scanner: {name: env}}\n    B: {scanner: {name: dotenv}}\n",
			packs:  deprecationPacks{scanners: map[string]string{"env": "use dotenv", "dotenv": ""}},
			want:   []string{`adder.yaml:4:19: scan "A": scanner "env" is deprecated: use dotenv`},
		},
		{
			name:    "a deprecated input needs a default",
			config:  "version: 1\ngenerators:\n  g:\n    files: [{template: t, path: p}]\n    inputs:\n      old: {deprecated: use new}\n      flag: {type: bool, deprecated: use another}\n",
			wantErr: []string{"adder.yaml:6:13 missing-field"},
		},
		{
			name:    "a deprecated option needs a default",
			config:  "version: 1\npacks:\n  caches: {pack: p}\n",
			packs:   deprecationPacks{packs: map[string]string{"p": "version: 1\noptions:\n  out: {deprecated: use dir}\n  on: {type: bool, deprecated: use dir}\n"}},
			wantErr: []string{"pack p: pack.yaml:3:9 missing-field"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved := deprecatedSettings
			deprecatedSettings = tt.settings
			t.Cleanup(func() { deprecatedSettings = saved })

			cfg, err := Parse("adder.yaml", []byte(tt.config), tt.packs)
			var problems []string
			for _, d := range diag.From(err) {
				problems = append(problems, d.Pos.String()+" "+string(d.Code))
			}
			if !slices.Equal(problems, tt.wantErr) {
				t.Fatalf("Parse() problems = %q, want %q\n%v", problems, tt.wantErr, err)
			}
			if err != nil {
				return
			}
			var got []string
			for _, d := range cfg.Warnings {
				if d.Code != diag.Deprecated {
					t.Errorf("warning %q has code %q, want %q", d.Message, d.Code, diag.Deprecated)
				}
				got = append(got, d.Error())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Warnings = %q, want %q", got, tt.want)
			}
		})
	}
}
