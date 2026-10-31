// Command adder is this repository's own build of adder, with the env-file scanner. Run it with go tool adder.
package main

import "typhonhill.dev/adder"

func main() {
	adder.Main(adder.Pack{Name: "local", Scanners: map[string]adder.Scanner{"env-file": envFile}})
}
