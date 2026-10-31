# Registry from functions

This example shows how adder keeps a hand-maintained list in sync with the code. A function scan finds every package under `drivers/` with an exported `New` function, and one template turns that list into a registry that constructs every driver. Adding or removing a driver package is all it takes to change the registry: nobody edits the list by hand, and nobody forgets to.

## How it works

- [`storage/storage.go`](storage/storage.go) defines the `Driver` interface, and [`drivers/disk`](drivers/disk) and [`drivers/memory`](drivers/memory) implement it, each with a `New` function.
- [`adder.yaml`](adder.yaml) declares the `Drivers` scan, which selects exported functions named `New` in `drivers/*`, and one output that renders [`templates/registry.go.tmpl`](templates/registry.go.tmpl).
- The template ranges over `.Drivers` and reads each item's `.ImportPath` and `.Package`. adder writes the result to [`drivers/drivers.gen.go`](drivers/drivers.gen.go), formats it with gofmt, and removes imports it does not use:

```go
func All() map[string]storage.Driver {
	return map[string]storage.Driver{
		"disk":   disk.New(),
		"memory": memory.New(),
	}
}
```

## Try it

Run the commands from this directory, with `go run ../../cmd/adder` in place of `adder` in a checkout of adder.

```console
$ adder check
$ go run ./cmd/demo
disk: hello
memory: hello
```

`adder check` exits 0 because the registry is current. Copy `drivers/memory` to `drivers/cache`, rename its package, and `adder check` reports `drivers/drivers.gen.go` as out of date until `adder derive` adds the new driver.

To create new drivers from a template instead of by copying, see [`scaffold-with-generator`](../scaffold-with-generator).
