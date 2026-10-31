# Custom scanner

This example shows a scanner written in Go, for input adder cannot read on its own. The built-in scans read Go declarations and list files. A scanner can read anything, and its items are rendered by templates like any others. Here it reads `KEY=value` settings from an `.env` file, so the application's defaults are typed Go constants derived from that file.

## How it works

- [`tools/adder/env.go`](tools/adder/env.go) is the scanner, an `adder.Scanner`. It decodes its options from the scan's `with` through `in.Decode`, reads each file from `in.FS`, and returns one item per setting, a map with `Key`, `Value`, and `File`.
- [`tools/adder/main.go`](tools/adder/main.go) builds this repository's own adder with the scanner, in a pack of its own: `adder.Pack{Name: "local", Scanners: ...}`. [`go.mod`](go.mod) declares it as a tool, so `go tool adder` runs it.
- [`adder.yaml`](adder.yaml) declares the `Settings` scan with `scanner: {name: env-file, with: {files: [config/defaults.env]}}`.
- [`templates/settings.go.tmpl`](templates/settings.go.tmpl) turns each key into a Go name. Keys are in capitals, so the template lowers them before `toGoPascalCase`, which then finds the words: `LISTEN_ADDR` is `ListenAddr`, and `CACHE_TTL` is `CacheTTL`. The result is [`config/settings.gen.go`](config/settings.gen.go):

```go
// Default settings, from defaults.env.
const (
	ListenAddr = ":8080"
	CacheTTL   = "5m"
	LogLevel   = "info"
)
```

A scanner reports a problem in its input as an `adder.Problem`, with the file and line, so it reads like adder's own. For a line without `=`:

```console
$ go tool adder check
config/defaults.env:2: want KEY=value [scanner]
	hint: comment lines start with #
```

## Try it

Run the commands from this directory:

```console
$ go tool adder check
$ go run ./cmd/demo
listen on :8080 with log level info
```

Add `REQUEST_TIMEOUT=30s` to `config/defaults.env`, and `adder derive` adds the `RequestTimeout` constant.

[`go.work`](go.work) builds the example with adder from this checkout, so it always matches the adder it documents. A repository of your own requires `typhonhill.dev/adder` in `go.mod` instead, and has no `go.work`. To share a scanner between repositories, put it in a pack, as in [`shared-pack`](../shared-pack).
