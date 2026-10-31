// Command adder is this repository's own build of adder, with the registry pack. Run it with go tool adder.
package main

import (
	"typhonhill.dev/adder"

	"example.com/shared-pack/registry"
)

func main() {
	adder.Main(registry.Pack)
}
