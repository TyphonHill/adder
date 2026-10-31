# Getting started

This guide adds adder to a module, derives a registry from the code, adds a generator for new code, and checks the result in CI. It takes about ten minutes. The finished repository is close to [`examples/registry-from-functions`](../examples/registry-from-functions) and [`examples/scaffold-with-generator`](../examples/scaffold-with-generator).

## Install

adder supports the two most recent Go releases. Add it as a tool of your module, so everyone runs the version `go.mod` pins:

```sh
go get -tool typhonhill.dev/adder/cmd/adder
go tool adder init
```

`adder init` writes a starter `adder.yaml` with every section commented out. It never overwrites an existing one. The rest of this guide writes `adder` for `go tool adder`.

## Derive a registry

Say the module has a `storage.Driver` interface, and one package per driver under `drivers/`, each with a `New` function:

```text
storage/storage.go       type Driver interface { ... }
drivers/disk/disk.go     func New() storage.Driver
drivers/memory/memory.go func New() storage.Driver
```

A **scan** selects declarations from the code, and an **output** renders a template with what the scans found. In `adder.yaml`:

```yaml
version: 1
derive:
  scans:
    Drivers:
      dirs: ["drivers/*"]
      funcs: {exported: true, name: ^New$}
  outputs:
    - template: templates/registry.go.tmpl
      path: drivers/drivers.gen.go
```

The template reads the scan by its name, `.Drivers`. Each item has fields such as `.Name`, `.Package`, and `.ImportPath`, and `.Module` is the module path from `go.mod`:

```gotemplate
package drivers

import (
	"{{ .Module }}/storage"
{{- range .Drivers }}
	"{{ .ImportPath }}"
{{- end }}
)

// All returns a new instance of every driver, by package name.
func All() map[string]storage.Driver {
	return map[string]storage.Driver{
{{- range .Drivers }}
		"{{ .Package }}": {{ .Package }}.New(),
{{- end }}
	}
}
```

```console
$ adder derive
created    drivers/drivers.gen.go
```

adder formats the file with gofmt, removes imports it does not use, and marks it with Go's generated-code header. Run `adder derive` again and nothing changes. Add a driver package and the registry follows. Commit the generated file, so the code builds without adder.

## Add a generator

A **generator** creates the files people then own, and updates the derived files in the same run. Add one to `adder.yaml`:

```yaml
generators:
  driver:
    description: Add a storage driver
    inputs:
      name: {help: Driver name, pattern: "[a-z][a-z0-9]*", unique: Drivers.Package}
    files:
      - template: templates/new/driver.go.tmpl
        path: "drivers/{{ .Inputs.name }}/{{ .Inputs.name }}.go"
```

The template reads the input as `.Inputs.name`. `pattern` and `unique` reject a bad name, or one a driver has already, before anything is written:

```console
$ adder new driver --name s3
created    drivers/s3/s3.go
updated    drivers/drivers.gen.go
```

`adder new driver --help` lists the inputs, and `--dry-run` shows what would change. To keep the template from changing by accident, give the generator [tests](configuration.md#testing-generators).

## Check in CI

`adder check` writes nothing and exits with status 3 when a derived file is out of date, such as after someone adds a driver by hand and forgets `adder derive`:

```console
$ adder check
would be updated    drivers/drivers.gen.go
adder: 1 derived file is out of date; run 'adder derive' to update it
```

Run it in CI with `--diff`, so the log shows what is stale, and run the generator tests with it:

```yaml
# .github/workflows/ci.yml
- run: go tool adder check --diff
- run: go tool adder test
```

## Next

- [Examples](../examples): ten small repositories, one feature each.
- [Configuration](configuration.md): every kind of scan, outputs with one file per item, and generator inputs.
- [Templates](templates.md): template data and functions.
