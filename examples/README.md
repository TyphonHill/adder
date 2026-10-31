# Examples

Each directory is a small, complete repository that shows one thing adder does. It has its own `go.mod`, `adder.yaml`, templates, and Go code, and the files adder derived from them, committed as a repository using adder would commit them. Each README says what the example shows, how it works, and what to try.

## Deriving files from code

| Example | Shows |
| --- | --- |
| [`registry-from-functions`](registry-from-functions) | A function scan, and a registry that constructs every package with a `New` function. |
| [`routes-from-directives`](routes-from-directives) | Declarations selected by a `//adder:route` comment, with the directive's arguments in the template. |
| [`enum-strings`](enum-strings) | A constant scan, and a `String` method for every `iota` enum, one per type with `groupBy`. |
| [`types-implementing-interface`](types-implementing-interface) | A type-aware scan that lists the types implementing an interface, by value or by pointer. |
| [`list-sql-migrations`](list-sql-migrations) | A file scan that lists the SQL files in a directory, in order. |
| [`file-per-item`](file-per-item) | An output with `each`, which writes one Markdown page per struct and removes the pages of deleted ones. |

## Creating files

| Example | Shows |
| --- | --- |
| [`scaffold-with-generator`](scaffold-with-generator) | `adder new` with validated inputs, an optional file, `--dry-run`, and golden-file tests. |

## Sharing and extending

| Example | Shows |
| --- | --- |
| [`shared-pack`](shared-pack) | A pack used twice with different options, in the repository's own build of adder. |
| [`custom-scanner`](custom-scanner) | A scanner written in Go, which reads settings from an `.env` file and reports problems at their line. |
| [`monorepo`](monorepo) | One `adder.yaml` per service, all checked at once with `adder check ./...`. |

## Running them

From a checkout of adder, change into an example's directory and run the adder in the checkout:

```sh
cd examples/registry-from-functions
go run ../../cmd/adder check   # exits 0: every derived file is current
```

`shared-pack` and `custom-scanner` build their own adder, so they run as `go tool adder` instead. The examples follow adder's `main` branch, which may be ahead of the latest release.

adder's tests run `adder check`, `adder test`, `go build ./...`, and `go vet ./...` in every example with adder from the same commit, so the examples always match the adder they document.
