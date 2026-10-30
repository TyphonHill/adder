# Packs

A pack shares derive outputs, generators, and templates between repositories, and a repository's own build of adder adds template functions and scanners written in Go.

- [`plugins/`](plugins) is a pack: [`pack.go`](plugins/pack.go) embeds [`pack.yaml`](plugins/pack.yaml) and its templates, and adds the `pluginTitle` function. It registers the plugins in the directories a use names, and adds new ones. In a real project it would be a module of its own, which other repositories require like any dependency.
- [`tools/adder/main.go`](tools/adder/main.go) is this repository's build of adder, with the pack and an `env-file` scanner of its own, in [`env.go`](tools/adder/env.go). `go tool adder` runs it.
- [`adder.yaml`](adder.yaml) uses the pack for caches, with its options under `with`, and declares a scan that runs the scanner on [`config/defaults.env`](config/defaults.env).

```console
$ go tool adder new plugin --name redis
created    caches/redis/redis.go
updated    caches/registry.gen.go
```

The scanner turns each `KEY=value` line into an item that [`templates/settings.go.tmpl`](templates/settings.go.tmpl) renders as a constant in [`config/settings.gen.go`](config/settings.gen.go), and reports a malformed line at its position:

```console
config/defaults.env:2: want KEY=value [scanner]
	hint: comment lines start with #
```

[`go.work`](go.work) builds the example with adder from this checkout. A repository of your own requires `typhonhill.dev/adder` in `go.mod` instead, and drops `go.work`.
