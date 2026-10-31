package config

import (
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"typhonhill.dev/adder/internal/diag"
)

// deprecatedSetting is a setting of adder.yaml or pack.yaml that still works, but is on its way out.
type deprecatedSetting struct {
	// Path is the setting's dotted path, with * for any key or index, such as derive.scans.*.funcs.name.
	Path string
	// Value, when set, deprecates only this value of the setting.
	Value string
	// Default deprecates relying on the setting's default: leaving the setting out where its parent is set.
	Default bool
	// Message says what to do instead, such as "use funcs.match".
	Message string
}

// deprecatedSettings lists the deprecated settings. It is empty until one is deprecated, as docs/compatibility.md
// describes, and tests add to it.
var deprecatedSettings []deprecatedSetting

// settingWarnings reports the deprecated settings that root, the parsed content of file, uses. Pack names the
// pack the file belongs to, if any.
func settingWarnings(file, pack string, root *yaml.Node) diag.List {
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	at := func(n *yaml.Node) diag.Pos { return diag.Pos{Pack: pack, File: file, Line: n.Line, Column: n.Column} }
	var warnings diag.List
	for _, s := range deprecatedSettings {
		parent, key := "", s.Path
		if i := strings.LastIndexByte(s.Path, '.'); i >= 0 {
			parent, key = s.Path[:i], s.Path[i+1:]
		}
		matchPath(root, parent, "", func(n *yaml.Node, path string) {
			if n.Kind != yaml.MappingNode {
				return
			}
			found := false
			for i := 0; i+1 < len(n.Content); i += 2 {
				k, v := n.Content[i], n.Content[i+1]
				if key != "*" && k.Value != key {
					continue
				}
				found = true
				switch setting := joinPath(path, k.Value); {
				case s.Default:
				case s.Value == "":
					warnings = append(warnings, Deprecation(at(k), setting+" is deprecated", s.Message))
				case v.Kind == yaml.ScalarNode && v.Value == s.Value:
					warnings = append(warnings, Deprecation(at(v), fmt.Sprintf("%s: %q is deprecated", setting, s.Value), s.Message))
				}
			}
			if s.Default && !found {
				warnings = append(warnings, Deprecation(at(n), joinPath(path, key)+" is left out, which relies on a default that is deprecated", s.Message))
			}
		})
	}
	return warnings
}

// matchPath calls fn with every node under n at path, a dotted path with * for any key or index, and the
// concrete path of each. At is n's own path.
func matchPath(n *yaml.Node, path, at string, fn func(*yaml.Node, string)) {
	if path == "" {
		fn(n, at)
		return
	}
	segment, rest, _ := strings.Cut(path, ".")
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			if k := n.Content[i]; segment == "*" || k.Value == segment {
				matchPath(n.Content[i+1], rest, joinPath(at, k.Value), fn)
			}
		}
	case yaml.SequenceNode:
		for i, child := range n.Content {
			if segment == "*" || segment == strconv.Itoa(i) {
				matchPath(child, rest, joinPath(at, strconv.Itoa(i)), fn)
			}
		}
	}
}

// Deprecation returns a warning that something deprecated was used, saying what to do instead.
func Deprecation(pos diag.Pos, message, instead string) *diag.Diagnostic {
	if instead != "" {
		message += ": " + instead
	}
	return &diag.Diagnostic{Code: diag.Deprecated, Pos: pos, Message: message}
}
