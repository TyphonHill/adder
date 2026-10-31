// Command demo stores and reads a value with every registered driver.
package main

import (
	"fmt"
	"log"
	"maps"
	"slices"

	"example.com/registry-from-functions/drivers"
)

func main() {
	all := drivers.All()
	for _, name := range slices.Sorted(maps.Keys(all)) {
		d := all[name]
		if err := d.Put("greeting", []byte("hello")); err != nil {
			log.Fatal(err)
		}
		value, err := d.Get("greeting")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: %s\n", name, value)
	}
}
