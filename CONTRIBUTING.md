# Contributing

Thanks for helping improve adder. This guide covers how to build and test it and the conventions the code follows. [docs/design.md](docs/design.md) explains how adder is built and why.

## Getting started

adder supports the two most recent Go releases. `go.mod` selects the newest toolchain automatically.

```sh
go test -race ./...
golangci-lint run
go mod tidy -diff
```

CI runs the same checks on both supported Go releases, plus `govulncheck`.

## Tests

### Every test is table-driven

Each test function declares its cases as a slice of structs with a `name` field and runs each case as a subtest:

```go
func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "camel case", in: "userAccount", want: "user_account"},
		{name: "acronym", in: "HTTPServer", want: "http_server"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toSnakeCase(tt.in); got != tt.want {
				t.Errorf("toSnakeCase(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
```

- Use a slice, not a map, so cases run in a stable order.
- Add a case to an existing table rather than writing a new test function for the same behavior.
- When cases need different preparation, give the struct an optional `setup` function rather than splitting the test.
- A test that cannot be table-driven needs a comment explaining why.

### Assert on what is stable

- Check diagnostic codes and positions, not message wording. Messages may improve in any release; codes are part of the [compatibility promise](docs/design.md#compatibility).
- Compare whole values, such as the complete list of scan items, rather than counts or single fields.

### Choose the right fixture

- Test logic against in-memory file systems (`fstest.MapFS`). Every pipeline step reads through `fs.FS` for this reason.
- Test real files, paths, and permissions in `internal/derive` and `internal/cli`, which use the fixture repositories under `internal/derive/testdata`.
- Generated output is compared with golden files. Run `go test ./internal/derive -update` to accept an intended change, and review the diff.
- The README quick start is the `basic` fixture. Change both together.

## Code

- Run `gofmt`; `golangci-lint run` checks formatting and imports.
- Report problems as `diag` diagnostics with a code and position. Add a new code only when no existing one fits, and document it in [docs/design.md](docs/design.md#4-structured-diagnostics).
- Keep comments short: say what code is for, not how it works, and only explain the *why* when it is not obvious.
- New behavior that affects generated output, configuration, or diagnostics needs a test, and the pull request should call it out.
