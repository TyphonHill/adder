# Routes from directives

This example shows how to mark the declarations a scan selects with a comment directive, instead of relying on names or signatures. Each HTTP handler says which method and path it serves, next to its code, and adder builds the route table from those directives. The arguments travel with the handler, so the route table never disagrees with it.

```go
// GetUser returns one user by ID.
//
//adder:route method=GET path=/users/{id}
func GetUser(w http.ResponseWriter, r *http.Request) {
```

## How it works

- [`api/users`](api/users/users.go) and [`api/orders`](api/orders/orders.go) hold the handlers. A helper without a directive, such as `format` in `users.go`, is left out.
- [`adder.yaml`](adder.yaml) declares the `Routes` scan with `funcs: {directive: route}`, which selects only functions marked `//adder:route`.
- [`templates/routes.go.tmpl`](templates/routes.go.tmpl) reads each directive's arguments with `index .Directives "route" "path"`, and groups the routes by package with `groupBy "ImportPath" .Routes`. The result is [`api/routes.gen.go`](api/routes.gen.go):

```go
func Register(mux *http.ServeMux) {
	// orders
	mux.HandleFunc("DELETE /orders/{id}", orders.CancelOrder)
	mux.HandleFunc("GET /orders", orders.ListOrders)
	// users
	mux.HandleFunc("POST /users", users.CreateUser)
	...
```

## Try it

Run the commands from this directory, with `go run ../../cmd/adder` in place of `adder` in a checkout of adder.

```console
$ adder check
$ go run ./cmd/server
```

The server listens on `:8080`, so `curl localhost:8080/users/42` prints `42`. To add a route, write a handler with its directive and run `adder derive`.
