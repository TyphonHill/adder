// Command migrate prints every migration in the order it would apply them.
package main

import (
	"fmt"
	"log"

	"example.com/list-sql-migrations/migrations"
)

func main() {
	for _, name := range migrations.Names {
		sql, err := migrations.Files.ReadFile(name)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("-- %s\n%s", name, sql)
	}
}
