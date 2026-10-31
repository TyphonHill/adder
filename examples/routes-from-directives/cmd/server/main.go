// Command server serves the example API on :8080.
package main

import (
	"log"
	"net/http"

	"example.com/routes-from-directives/api"
)

func main() {
	mux := http.NewServeMux()
	api.Register(mux)
	log.Fatal(http.ListenAndServe(":8080", mux))
}
