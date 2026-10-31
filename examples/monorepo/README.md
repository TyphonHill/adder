# Monorepo

This example shows a repository with several `adder.yaml` files, one per service, and how to run adder over all of them at once. Each service is a Go module of its own and derives its own files from its own scans, and a single `adder check ./...` at the top checks every one, the way `go test ./...` tests every package.

## How it works

- [`services/billing`](services/billing) and [`services/search`](services/search) each have a `go.mod`, an `adder.yaml`, and a template. Each `adder.yaml` lists the service's handlers in `handlers/names.gen.go`.
- The directory holding an `adder.yaml` is the root for its scans, outputs, and templates, so each service's paths read as if it were a repository of its own: `dirs: [handlers]`, not `dirs: [services/billing/handlers]`.
- A command given directory patterns, such as `./...` or `./services/search`, runs over every `adder.yaml` they match, one after another. A problem in one service does not stop the others, and the paths it prints start from the current directory.

## Try it

Run the commands from this directory, with `go run ../../cmd/adder` in place of `adder` in a checkout of adder.

```console
$ adder derive ./...
$ adder check ./...
```

Add a handler to `services/search/handlers/handlers.go`, and the check names the file it would update:

```console
$ adder check ./...
would be updated    services/search/handlers/names.gen.go
adder: 1 derived file is out of date; run 'adder derive' to update it
```

Without a pattern, adder uses the nearest `adder.yaml`, found from the current directory as `go` finds `go.mod`, so `cd services/search && adder derive` works on that service alone. If the services share their templates, [`shared-pack`](../shared-pack) shows how to write them once.
