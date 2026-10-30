// Package plugins is a pack that registers the plugins in a set of directories, one package each, and adds new
// ones. A repository uses it once per kind of plugin. In a real project it would be a module of its own, so that
// unrelated repositories can share it.
package plugins

import (
	"embed"
	"strings"

	"typhonhill.dev/adder"
)

//go:embed pack.yaml templates
var files embed.FS

// Pack is the plugins pack, for adder.Main.
var Pack = adder.Pack{
	Name: "plugins",
	FS:   files,
	Funcs: map[string]any{
		// pluginTitle names a plugin in documentation: memory is Memory.
		"pluginTitle": func(name string) string { return strings.ToUpper(name[:1]) + name[1:] },
	},
}
