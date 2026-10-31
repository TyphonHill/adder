# Types implementing an interface

This example shows a type-aware scan, which asks the Go type checker which types implement an interface, rather than matching names. Every health check in [`checks/`](checks/checks.go) that implements `health.Checker` is listed, whatever it is called, and a type that has only some of the methods is left out.

## How it works

- [`health/health.go`](health/health.go) defines the `Checker` interface, with `Name` and `Check` methods.
- [`checks/checks.go`](checks/checks.go) has three types. `Disk` implements `Checker` with value receivers. `Database` has pointer receivers, so only `*Database` implements it. `Upstream` has no `Name` method, so it does not.
- [`adder.yaml`](adder.yaml) declares the `Checks` scan with `typeCheck: true` and `types: {implements: example.com/types-implementing-interface/health.Checker}`. The interface is written with its full import path.
- Each item's `.Pointer` is true when only a pointer to the type implements the interface, so [`templates/all.go.tmpl`](templates/all.go.tmpl) writes `&Database{}` but `Disk{}`. The result is [`checks/all.gen.go`](checks/all.gen.go):

```go
func All() []health.Checker {
	return []health.Checker{
		&Database{},
		Disk{},
	}
}
```

`typeCheck` loads the packages with the `go` command, so it is slower than the syntax-only scans in the other examples. Use it when names are not enough, as here.

## Try it

Run the commands from this directory, with `go run ../../cmd/adder` in place of `adder` in a checkout of adder.

```console
$ adder check
$ go run ./cmd/demo
database: <nil>
disk: <nil>
```

Add a `Name` method to `Upstream`, and `adder derive` adds it to the list.
