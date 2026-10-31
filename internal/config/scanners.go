package config

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"typhonhill.dev/adder/internal/diag"
)

// ScannerMatch has a scan run a custom scanner from a pack, with the settings it decodes itself.
type ScannerMatch struct {
	Name string    `yaml:"name"`
	With yaml.Node `yaml:"with"`

	// Pos is where the scanner is named.
	Pos diag.Pos `yaml:"-"`
}

var unmarshalerType = reflect.TypeFor[yaml.Unmarshaler]()

// Decode decodes the scanner's settings into v, a pointer, rejecting fields v does not have. Problems are
// reported at their position in the configuration. Without settings, v is left as it is.
func (m ScannerMatch) Decode(v any) error {
	if m.With.Kind == 0 {
		return nil
	}
	at := func(n *yaml.Node) diag.Pos {
		return diag.Pos{Pack: m.Pos.Pack, File: m.Pos.File, Line: n.Line, Column: n.Column}
	}
	errs := unknownFields(&m.With, reflect.TypeOf(v), at)
	if len(errs) > 0 {
		return errs
	}
	if err := m.With.Decode(v); err != nil {
		list := diag.From(decodeError(m.Pos.File, err))
		for _, d := range list {
			d.Pos.Pack = m.Pos.Pack
		}
		return list
	}
	return nil
}

// unknownFields reports the keys in n that the type t, which n decodes into, has no field for.
func unknownFields(n *yaml.Node, t reflect.Type, at func(*yaml.Node) diag.Pos) diag.List {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || reflect.PointerTo(t).Implements(unmarshalerType) {
		return nil
	}
	var errs diag.List
	switch {
	case n.Kind == yaml.AliasNode:
		return unknownFields(n.Alias, t, at)
	case n.Kind == yaml.MappingNode && t.Kind() == reflect.Struct:
		fields := yamlFields(t)
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, value := n.Content[i], n.Content[i+1]
			field, ok := fields[key.Value]
			if !ok {
				errs = append(errs, &diag.Diagnostic{
					Code:    diag.UnknownField,
					Pos:     at(key),
					Message: fmt.Sprintf("unknown field %q", key.Value),
					Hint:    "known fields: " + strings.Join(slices.Sorted(maps.Keys(fields)), ", "),
				})
				continue
			}
			errs = append(errs, unknownFields(value, field, at)...)
		}
	case n.Kind == yaml.MappingNode && t.Kind() == reflect.Map:
		for i := 1; i < len(n.Content); i += 2 {
			errs = append(errs, unknownFields(n.Content[i], t.Elem(), at)...)
		}
	case n.Kind == yaml.SequenceNode && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array):
		for _, child := range n.Content {
			errs = append(errs, unknownFields(child, t.Elem(), at)...)
		}
	}
	return errs
}

// yamlFields returns the type of each field of the struct type t by its YAML key, as the YAML decoder names
// them: by tag, or by the lowercased field name, with inline structs' fields among them.
func yamlFields(t reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}
		name, opts, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		switch {
		case name == "-":
		case slices.Contains(strings.Split(opts, ","), "inline") && f.Type.Kind() == reflect.Struct:
			maps.Copy(fields, yamlFields(f.Type))
		case name == "":
			fields[strings.ToLower(f.Name)] = f.Type
		default:
			fields[name] = f.Type
		}
	}
	return fields
}

// checkScanners reports scans that name a scanner no pack provides.
func (c *Config) checkScanners(packs Packs) diag.List {
	var errs diag.List
	for _, name := range slices.Sorted(maps.Keys(c.Derive.Scans)) {
		s := c.Derive.Scans[name]
		if s.Scanner == nil || s.Scanner.Name == "" {
			continue
		}
		if packs != nil && packs.HasScanner(s.Scanner.Name) {
			if instead := packs.ScannerDeprecation(s.Scanner.Name); instead != "" {
				c.Warnings = append(c.Warnings, deprecation(s.Scanner.Pos, fmt.Sprintf("scan %q: scanner %q is deprecated", name, s.Scanner.Name), instead))
			}
			continue
		}
		errs = append(errs, &diag.Diagnostic{
			Code:    diag.UnknownScanner,
			Pos:     s.Scanner.Pos,
			Message: fmt.Sprintf("scan %q: adder was built without scanner %q", name, s.Scanner.Name),
			Hint:    "scanners come from packs in a repository's own build of adder, such as go tool adder",
		})
	}
	return errs
}
