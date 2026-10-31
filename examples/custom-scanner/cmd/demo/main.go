// Command demo prints the default settings.
package main

import (
	"fmt"

	"example.com/custom-scanner/config"
)

func main() {
	fmt.Println("listen on", config.ListenAddr, "with log level", config.LogLevel)
}
