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

- Check diagnostic codes and positions, not message wording. Messages may improve in any release; codes are part of the [compatibility promise](docs/compatibility.md).
- Compare whole values, such as the complete list of scan items, rather than counts or single fields.

### Choose the right fixture

- Test logic against in-memory file systems (`fstest.MapFS`). Every pipeline step reads through `fs.FS` for this reason.
- Test real files, paths, and permissions in `internal/derive` and `internal/cli`, which use the fixture repositories under `internal/derive/testdata`.
- Generated output is compared with golden files. Run `go test ./internal/derive -update` to accept an intended change, and review the diff.
- The README's example comes from `examples/registry-from-functions` and `examples/scaffold-with-generator`, and its output was copied from running them. Change them together.

### Test the command line end to end

Scripts in `cmd/adder/testdata/script` run the real `adder` binary against small repositories written inline, using [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript). Add a script, or a section to one, for any change to what a command prints or how it exits.

- `exits 3 adder check` runs a command and checks its exit status. `exec` and `! exec` only tell success from failure.
- Compare output with `cmp stdout name.out` against a file in the script. Run `go test ./cmd/adder -update` to fill in or rewrite those files, then review the diff.
- Positions in problems use the operating system's separator, so match them with a pattern such as `stderr '^drivers[/\\]x\.go'` rather than `cmp`.

### Fuzz parsers

Fuzz tests cover configuration loading, header parsing, directive parsing, and import pruning. Their seed inputs run with every `go test`, and a weekly workflow fuzzes each for five minutes. To fuzz locally:

```sh
go test ./internal/imports -run '^$' -fuzz '^FuzzPrune$' -fuzztime 1m
```

A failure is saved under the package's `testdata/fuzz`. Commit it with the fix, so it stays a regression test.

### Keep derive fast

`BenchmarkDerive` runs `derive` over a generated repository of 1,000 packages. Its budget is 500 ms on GitHub's runners, and CI fails above four times that with `go test ./internal/derive -run '^TestBudget$' -budget`. Compare a change that may affect speed with `go test ./internal/derive -run '^$' -bench BenchmarkDerive -count 10` before and after, using `benchstat`.

## Code

- Run `gofmt`; `golangci-lint run` checks formatting and imports.
- Report problems as `diag` diagnostics with a code and position. Add a new code only when no existing one fits, and document it in [docs/design.md](docs/design.md#4-structured-diagnostics).
- Keep comments short: say what code is for, not how it works, and only explain the *why* when it is not obvious.
- New behavior that affects generated output, configuration, or diagnostics needs a test, and the pull request should call it out.

## Releasing

Releases are tagged by hand from `main`. A Go module version can never change once anyone has downloaded it, because the checksum database records it, so a mistake in a release is fixed with a new patch release, never by moving or re-creating its tag.

1. Wait until every check on `main` has passed, Windows and macOS included. Never tag a commit whose checks are still running.
2. In a pull request:
   - Move the `[Unreleased]` entries in `CHANGELOG.md` under `## [X.Y.Z] - YYYY-MM-DD`, leave an empty `[Unreleased]` above it, and update the compare links at the bottom.
   - Remove `(planned, vX.Y)` from the headings of the decisions in `docs/design.md` that shipped, and update the links to them in the roadmap, the design document, and the README.
   - Mark the shipped roadmap rows ✅.
3. Merge it once every check passes, then tag the merge commit `vX.Y.Z` and publish a GitHub release whose notes are the changelog section, with a link to the full changelog.
