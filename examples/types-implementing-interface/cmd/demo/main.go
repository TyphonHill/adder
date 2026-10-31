// Command demo runs every health check.
package main

import (
	"fmt"

	"example.com/types-implementing-interface/checks"
)

func main() {
	for _, c := range checks.All() {
		fmt.Printf("%s: %v\n", c.Name(), c.Check())
	}
}
