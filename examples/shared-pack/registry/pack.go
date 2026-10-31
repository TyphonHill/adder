// Package registry is a pack that lists the packages in a set of directories that have an exported New
// function. In a real project it would be a module of its own, so that unrelated repositories can share it.
package registry

import (
	"embed"

	"typhonhill.dev/adder"
)

//go:embed pack.yaml templates
var files embed.FS

// Pack is the registry pack, for adder.Main.
var Pack = adder.Pack{Name: "registry", FS: files}
