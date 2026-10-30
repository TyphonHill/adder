package config

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"text/template"

	"github.com/TyphonHill/adder/internal/diag"
)

// scannerPacks provides the scanners it lists, and no packs.
type scannerPacks []string

func (scannerPacks) FS(string) (fs.FS, bool)       { return nil, false }
func (scannerPacks) Funcs() template.FuncMap       { return nil }
func (p scannerPacks) HasScanner(name string) bool { return slices.Contains(p, name) }

func TestScannerScans(t *testing.T) {
	tests := []struct {
		name  string
		scans string
		want  []string
	}{
		{name: "a scanner without dirs", scans: "R: {scanner: {name: routes, with: {prefix: /v1}}}"},
		{name: "a scanner with dirs", scans: "R: {dirs: [\"api/*\"], exclude: [api/old], scanner: {name: routes}}"},
		{name: "a scanner adder was not built with", scans: "R: {scanner: {name: nope}}", want: []string{"adder.yaml:4:19 unknown-scanner"}},
		{name: "a scanner without a name", scans: "R: {scanner: {with: {}}}", want: []string{"adder.yaml:4:9 missing-field"}},
		{name: "a scanner and funcs", scans: "R: {dirs: [x], funcs: {}, scanner: {name: routes}}", want: []string{"adder.yaml:4:31 invalid-value"}},
		{
			name:  "settings a scanner does not take",
			scans: "R: {scanner: {name: routes}, includeTests: true, buildTags: [x], typeCheck: true}",
			want:  []string{"adder.yaml:4:34 invalid-value", "adder.yaml:4:54 invalid-value", "adder.yaml:4:70 invalid-value"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse("adder.yaml", []byte("version: 1\nderive:\n  scans:\n    "+tt.scans+"\n"), scannerPacks{"routes"})
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

func TestScannerDecode(t *testing.T) {
	type route struct {
		Path    string
		Methods []string `yaml:"methods"`
	}
	type settings struct {
		Prefix string           `yaml:"prefix"`
		Routes []route          `yaml:"routes"`
		Named  map[string]route `yaml:"named"`
		Name   Pattern          `yaml:"name"`
		Common struct {
			Owner string `yaml:"owner"`
		} `yaml:",inline"`
		Skipped string `yaml:"-"`
	}
	tests := []struct {
		name    string
		with    string
		want    settings
		wantErr []string
	}{
		{name: "no settings", with: "", want: settings{}},
		{name: "every kind of field", with: "{prefix: /v1, owner: api, routes: [{path: /a, methods: [GET]}], named: {x: {path: /x}}, name: ^R}", want: settings{Prefix: "/v1", Routes: []route{{Path: "/a", Methods: []string{"GET"}}}, Named: map[string]route{"x": {Path: "/x"}}}},
		{name: "an unknown field", with: "{prefx: /v1}", wantErr: []string{"adder.yaml:4:40 unknown-field"}},
		{name: "unknown fields deep inside", with: "{routes: [{path: /a, verb: GET}], named: {x: {pth: /x}}, Skipped: x}", wantErr: []string{"adder.yaml:4:60 unknown-field", "adder.yaml:4:85 unknown-field", "adder.yaml:4:96 unknown-field"}},
		{name: "a value of the wrong type", with: "{routes: x}", wantErr: []string{"adder.yaml:4 invalid-value"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := "{name: routes}"
			if tt.with != "" {
				scanner = "{name: routes, with: " + tt.with + "}"
			}
			cfg, err := Parse("adder.yaml", []byte("version: 1\nderive:\n  scans:\n    R: {scanner: "+scanner+"}\n"), scannerPacks{"routes"})
			if err != nil {
				t.Fatal(err)
			}
			var got settings
			err = cfg.Derive.Scans["R"].Scanner.Decode(&got)
			var errs []string
			for _, d := range diag.From(err) {
				errs = append(errs, d.Pos.String()+" "+string(d.Code))
			}
			if !slices.Equal(errs, tt.wantErr) {
				t.Fatalf("Decode() problems = %q, want %q\n%v", errs, tt.wantErr, err)
			}
			if err != nil {
				return
			}
			got.Name = Pattern{}
			if got.Prefix != tt.want.Prefix || len(got.Routes) != len(tt.want.Routes) || len(got.Named) != len(tt.want.Named) {
				t.Errorf("Decode() = %+v, want %+v", got, tt.want)
			}
			if strings.Contains(tt.with, "owner") && got.Common.Owner != "api" {
				t.Errorf("inline field = %q", got.Common.Owner)
			}
		})
	}
}
