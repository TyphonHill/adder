// Command demo prints the caches and queues the registries list.
package main

import (
	"fmt"

	"example.com/shared-pack/caches"
	"example.com/shared-pack/queues"
)

func main() {
	fmt.Println("caches:", caches.Names)
	fmt.Println("queues:", queues.Names)
}
