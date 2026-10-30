# Registry

A registry of storage drivers that stays in sync with the code. Each package under `drivers/` with an exported `New` function is a driver, and adder writes `drivers/drivers.gen.go`, which constructs every one of them:

```go
func All() map[string]storage.Driver {
	return map[string]storage.Driver{
		"disk":   disk.New(),
		"memory": memory.New(),
	}
}
```

Add a driver with the generator, and the registry includes it in the same run:

```console
$ adder new driver --name s3 --summary "an S3 bucket"
created    drivers/s3/s3.go
updated    drivers/drivers.gen.go

Implement Get and Put in drivers/s3/s3.go. drivers.All includes it already.
```

- [`adder.yaml`](adder.yaml) declares the `Drivers` scan, the output, and the `driver` generator, whose `unique` input rejects a driver name that exists already.
- [`templates/registry.go.tmpl`](templates/registry.go.tmpl) renders the registry. adder formats it with gofmt and removes unused imports.
- The generator's test renders it with `name: example` and compares the result with [`testdata/generators/driver/plain`](testdata/generators/driver/plain). Run it with `adder test`.
- `go run ./cmd/demo` stores and reads a value with every driver.
