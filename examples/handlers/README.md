# Handlers

HTTP routes declared next to their handlers, in comment directives:

```go
// GetUser returns one user by ID.
//
//adder:route method=GET path=/users/{id}
func GetUser(w http.ResponseWriter, r *http.Request) {
```

adder writes two kinds of file from them:

- [`api/routes.gen.go`](api/routes.gen.go) registers every route on an `http.ServeMux`, grouped by package with `groupBy`.
- [`docs/routes/`](docs/routes) holds one Markdown page per route, from an output with `each`. Deleting a route's directive removes its page on the next `adder derive`, and `adder check` reports the page as stale until then.

The scan selects only functions with the `route` directive, so helpers in the same packages are left out, and the template reads the directive's arguments with `index .Directives "route" "path"`. Adding a route is a matter of writing the handler with its directive and running `adder derive`.

`go run ./cmd/server` serves the routes on `:8080`.
