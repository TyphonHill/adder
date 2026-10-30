// Command adder is this repository's own build of adder, with the plugins pack and a scanner of its own. Run it
// with go tool adder.
package main

import (
	"typhonhill.dev/adder"

	"example.com/packs/plugins"
)

func main() {
	adder.Main(
		plugins.Pack,
		adder.Pack{Name: "local", Scanners: map[string]adder.Scanner{"env-file": envFile}},
	)
}
