// Command app prints its caches and its default settings.
package main

import (
	"fmt"

	"example.com/packs/caches"
	"example.com/packs/config"
)

func main() {
	fmt.Println("caches:", caches.Names)
	fmt.Println("listening on", config.ListenAddr)
}
