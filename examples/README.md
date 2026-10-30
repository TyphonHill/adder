# Examples

Each directory is a complete repository with its own `go.mod`, `adder.yaml`, templates, Go code, and the files adder derived from them, committed as a repository using adder would commit them.

| Example | Shows |
| --- | --- |
| [`registry`](registry) | A registry of storage drivers derived from their `New` functions, a generator that adds a driver, and a generator test. |
| [`handlers`](handlers) | HTTP routes declared in comment directives, a route table grouped by package, and one page per route. |
| [`packs`](packs) | A pack that registers plugins, used by an application that builds its own adder with it, plus a template function and a custom scanner. |

To try one from a checkout of adder, change into its directory and run the adder in the checkout:

```sh
cd examples/registry
go run ../../cmd/adder check   # exits 0: every derived file is current
```

`packs` builds its own adder, so it runs as `go tool adder` instead. The examples follow adder's `main` branch, which may be ahead of the latest release.

adder's tests run `adder check`, `adder test`, and `go build ./...` in each example with adder from the same commit, so the examples always match the adder they document.
