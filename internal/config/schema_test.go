package config

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/txtar"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"

	"typhonhill.dev/adder/internal/diag"
)

var update = flag.Bool("update", false, "rewrite schema/pack-v1.json from schema/adder-v1.json")

const (
	adderSchema = "../../schema/adder-v1.json"
	packSchema  = "../../schema/pack-v1.json"
)

func readSchema(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return doc
}

// TestSchemaMatchesTypes checks that the schemas have a property for every field of the configuration types, and
// no others, so that neither drifts from the other.
func TestSchemaMatchesTypes(t *testing.T) {
	tests := []struct {
		schema string
		typ    reflect.Type
		skip   []string // field paths the schema leaves out, as pack.yaml leaves out generator tests
	}{
		{schema: adderSchema, typ: reflect.TypeFor[Config]()},
		{schema: packSchema, typ: reflect.TypeFor[packFile](), skip: []string{"generators.*.tests"}},
	}
	for _, tt := range tests {
		t.Run(filepath.Base(tt.schema), func(t *testing.T) {
			doc := readSchema(t, tt.schema)
			compareType(t, doc, doc, tt.typ, "", tt.skip)
		})
	}
}

var unmarshaler = reflect.TypeFor[yaml.Unmarshaler]()

// compareType compares the fields of t, at path, with the properties of the schema s, within doc.
func compareType(t *testing.T, doc, s map[string]any, typ reflect.Type, path string, skip []string) {
	t.Helper()
	s = resolve(doc, s)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if reflect.PointerTo(typ).Implements(unmarshaler) || typ == reflect.TypeFor[yaml.Node]() {
		return // decoded from a scalar or kept as written
	}
	switch typ.Kind() {
	case reflect.Map:
		if sub, ok := s["additionalProperties"].(map[string]any); ok {
			compareType(t, doc, sub, typ.Elem(), path+".*", skip)
		}
	case reflect.Slice:
		if sub, ok := s["items"].(map[string]any); ok {
			compareType(t, doc, sub, typ.Elem(), path+".*", skip)
		}
	case reflect.Struct:
		props, _ := s["properties"].(map[string]any)
		var fields []string
		for field := range typ.Fields() {
			name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
			at := strings.TrimPrefix(path+"."+name, ".")
			if name == "" || name == "-" || slices.Contains(skip, at) {
				continue
			}
			fields = append(fields, name)
			if sub, ok := props[name].(map[string]any); ok {
				compareType(t, doc, sub, field.Type, at, skip)
			}
		}
		slices.Sort(fields)
		if keys := slices.Sorted(maps.Keys(props)); !slices.Equal(keys, fields) {
			t.Errorf("%s: schema properties = %q, want the fields of %s: %q", cmpPath(path), keys, typ.Name(), fields)
		}
	}
}

func cmpPath(path string) string {
	if path == "" {
		return "the file"
	}
	return strings.TrimPrefix(path, ".")
}

// resolve follows $ref, a sole allOf, and the anyOf the pack schema wraps around values, to the schema they stand
// for.
func resolve(doc, s map[string]any) map[string]any {
	for {
		switch {
		case s["$ref"] != nil:
			name := strings.TrimPrefix(s["$ref"].(string), "#/definitions/")
			s = doc["definitions"].(map[string]any)[name].(map[string]any)
		case s["properties"] == nil && s["items"] == nil && s["additionalProperties"] == nil && len(anyOf(s)) > 0:
			s = anyOf(s)[0].(map[string]any)
		default:
			return s
		}
	}
}

func anyOf(s map[string]any) []any {
	if list, ok := s["allOf"].([]any); ok {
		return list
	}
	list, _ := s["anyOf"].([]any)
	return list
}

// TestPackSchema checks that the pack.yaml schema is the adder.yaml schema with what a pack.yaml can hold, in
// which every value may instead be an option's value, as {option: name}. Run with -update to rewrite it.
func TestPackSchema(t *testing.T) {
	want, err := json.MarshalIndent(makePackSchema(readSchema(t, adderSchema)), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if *update {
		if err := os.WriteFile(packSchema, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := os.ReadFile(packSchema); err != nil || !bytes.Equal(got, want) {
		t.Errorf("%s is out of date (%v); run go test ./internal/config -run TestPackSchema -update", packSchema, err)
	}
}

// makePackSchema derives the pack.yaml schema from the adder.yaml schema.
func makePackSchema(adder map[string]any) map[string]any {
	defs := clone(adder["definitions"]).(map[string]any)
	delete(defs, "use")
	delete(defs["generator"].(map[string]any)["properties"].(map[string]any), "tests")
	for name, def := range defs {
		if name != "partials" {
			defs[name] = optionable(def)
		}
	}
	defs["option"] = map[string]any{
		"description":          "Takes the value of one of the pack's options.",
		"type":                 "object",
		"required":             []any{"option"},
		"additionalProperties": false,
		"properties":           map[string]any{"option": map[string]any{"$ref": "#/definitions/inputName"}},
	}
	defs["packOption"] = map[string]any{
		"description":          "A setting the pack takes from each use, as .Options.<name> in its templates.",
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"type":       map[string]any{"description": "string, the default, bool, or list.", "enum": []any{"string", "bool", "list"}},
			"help":       map[string]any{"description": "Shown with problems about the option.", "type": "string"},
			"default":    map[string]any{"description": "Makes the option optional. Bool options default to false."},
			"pattern":    map[string]any{"description": "A regular expression the whole value, or each value of a list, must match.", "type": "string"},
			"values":     map[string]any{"description": "The allowed values, or the values a list may hold.", "type": "array", "items": map[string]any{"type": "string"}},
			"deprecated": map[string]any{"description": "Marks the option as on its way out, saying what to use instead. A deprecated string or list option needs a default.", "type": "string"},
		},
	}
	return map[string]any{
		"$schema":              adder["$schema"],
		"$id":                  strings.Replace(adder["$id"].(string), "adder-v1.json", "pack-v1.json", 1),
		"$comment":             "Generated from adder-v1.json by TestPackSchema in internal/config. Do not edit.",
		"title":                "pack.yaml",
		"description":          "adder pack configuration, version 1: https://github.com/typhonhilldev/adder/blob/main/docs/extending.md",
		"type":                 "object",
		"required":             []any{"version"},
		"additionalProperties": false,
		"properties": map[string]any{
			"version": adder["properties"].(map[string]any)["version"],
			"options": map[string]any{
				"description":          "The settings each use gives under with.",
				"type":                 []any{"object", "null"},
				"propertyNames":        map[string]any{"$ref": "#/definitions/inputName"},
				"additionalProperties": map[string]any{"$ref": "#/definitions/packOption"},
			},
			"partials":   map[string]any{"$ref": "#/definitions/partials"},
			"derive":     map[string]any{"$ref": "#/definitions/derive"},
			"generators": map[string]any{"$ref": "#/definitions/generators"},
		},
		"definitions": defs,
	}
}

// optionable makes every value below the schema s, though not s itself, either what s allows or an option.
func optionable(s any) any {
	m, ok := s.(map[string]any)
	if !ok {
		return s
	}
	or := func(v any) any {
		return map[string]any{"anyOf": []any{optionable(v), map[string]any{"$ref": "#/definitions/option"}}}
	}
	if props, ok := m["properties"].(map[string]any); ok {
		for k, v := range props {
			props[k] = or(v)
		}
	}
	if items, ok := m["items"].(map[string]any); ok {
		m["items"] = or(items)
	}
	if extra, ok := m["additionalProperties"].(map[string]any); ok {
		m["additionalProperties"] = or(extra)
	}
	for _, key := range []string{"allOf", "anyOf"} {
		if list, ok := m[key].([]any); ok {
			for i := range list {
				list[i] = optionable(list[i])
			}
		}
	}
	return m
}

func clone(v any) any {
	data, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(data, &out)
	return out
}

// compile compiles the schema at path, with the files it refers to by $id read from disk.
func compile(t *testing.T, path string) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	for _, p := range []string{adderSchema, packSchema} {
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := c.AddResource(readSchema(t, p)["$id"].(string), doc); err != nil {
			t.Fatal(err)
		}
	}
	s, err := c.Compile(readSchema(t, path)["$id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// instance converts YAML to the JSON values a schema validates.
func instance(t *testing.T, data []byte) any {
	t.Helper()
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// TestSchemaAcceptsConfigs checks that the schemas accept every adder.yaml and pack.yaml in the repository's
// examples, fixtures, and scripts that adder itself loads.
func TestSchemaAcceptsConfigs(t *testing.T) {
	adder, pack := compile(t, adderSchema), compile(t, packSchema)
	found := map[string]int{}
	for name, data := range configFiles(t) {
		switch filepath.Base(name) {
		case FileName:
			if !loads(data) {
				continue
			}
			found[FileName]++
			if err := adder.Validate(instance(t, data)); err != nil {
				t.Errorf("%s: adder loads it, but the schema rejects it:\n%v", name, err)
			}
		case PackFileName:
			found[PackFileName]++
			if err := pack.Validate(instance(t, data)); err != nil {
				t.Errorf("%s: the schema rejects it:\n%v", name, err)
			}
		}
	}
	if found[FileName] < 15 || found[PackFileName] < 3 {
		t.Errorf("found %d adder.yaml and %d pack.yaml files, want more: the search is broken", found[FileName], found[PackFileName])
	}
}

// loads reports whether adder loads the configuration, setting aside the packs and scanners a test build lacks.
func loads(data []byte) bool {
	_, err := Parse(FileName, data, nil)
	for _, d := range diag.From(err) {
		if d.Code != diag.UnknownPack && d.Code != diag.UnknownScanner {
			return false
		}
	}
	return true
}

// configFiles returns the configuration files under the repository, by a name for each, including those in the
// txtar archives of the scripts.
func configFiles(t *testing.T) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	err := filepath.WalkDir("../..", func(name string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules"):
			return filepath.SkipDir
		case d.Name() == FileName || d.Name() == PackFileName:
			data, err := os.ReadFile(name)
			files[name] = data
			return err
		case strings.HasSuffix(name, ".txtar"):
			archive, err := txtar.ParseFile(name)
			if err != nil {
				return err
			}
			for _, f := range archive.Files {
				if base := filepath.Base(f.Name); base == FileName || base == PackFileName {
					files[name+":"+f.Name] = f.Data
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestSchemaRejects checks that the schemas catch common mistakes.
func TestSchemaRejects(t *testing.T) {
	adder, pack := compile(t, adderSchema), compile(t, packSchema)
	tests := []struct {
		name   string
		pack   bool
		config string
	}{
		{name: "no version", config: "derive: {}"},
		{name: "another version", config: "version: 2"},
		{name: "an unknown key", config: "version: 1\nderive: {scanz: {}}"},
		{name: "a reserved scan name", config: "version: 1\nderive: {scans: {Module: {dirs: [x]}}}"},
		{name: "a scan name that is not an identifier", config: "version: 1\nderive: {scans: {my-scan: {dirs: [x]}}}"},
		{name: "an unknown kind", config: "version: 1\nderive: {scans: {A: {types: {kind: type}}}}"},
		{name: "an output without a path", config: "version: 1\nderive: {outputs: [{template: t}]}"},
		{name: "a generator name in capitals", config: "version: 1\ngenerators: {Driver: {files: [{template: t, path: p}]}}"},
		{name: "a generator without files", config: "version: 1\ngenerators: {driver: {}}"},
		{name: "an input name in kebab-case", config: "version: 1\ngenerators: {driver: {inputs: {package-name: {}}, files: [{template: t, path: p}]}}"},
		{name: "an unknown ifExists", config: "version: 1\ngenerators: {driver: {files: [{template: t, path: p, ifExists: overwrite}]}}"},
		{name: "a use without a pack", config: "version: 1\npacks: {drivers: {with: {}}}"},
		{name: "vars in a pack", pack: true, config: "version: 1\nvars: {A: b}"},
		{name: "a use in a pack", pack: true, config: "version: 1\npacks: {a: {pack: b}}"},
		{name: "tests in a pack's generator", pack: true, config: "version: 1\ngenerators: {g: {files: [{template: t, path: p}], tests: []}}"},
		{name: "an option placeholder with another key", pack: true, config: "version: 1\nderive: {scans: {A: {dirs: {option: dirs, x: y}}}}"},
		{name: "an unknown option type", pack: true, config: "version: 1\noptions: {dirs: {type: map}}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := adder
			if tt.pack {
				s = pack
			}
			if err := s.Validate(instance(t, []byte(tt.config))); err == nil {
				t.Errorf("the schema accepts:\n%s", tt.config)
			}
		})
	}
}
