# Changelog

All notable changes to adder are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and adder uses [semantic versioning](https://semver.org/). Before v1.0.0, a minor release may include breaking changes; they are listed under **Changed** or **Removed**.

## [Unreleased]

## [0.4.0] - 2026-10-30

### Added

- Constant and variable scans, with a `values` block filtering by `kind`, `exported`, `name`, `type`, and `directive`. Items gain `.Type`, `.Value`, and `.Iota`, and a `const` group's type carries to the names that repeat it, so `iota` enums work.
- Monorepos: `derive`, `check`, `list`, and `test` take directory patterns such as `./...` and run every `adder.yaml` they match, with paths shown from the current directory and the most serious exit status. Wildcards and the search for stale files stop at nested `adder.yaml` and `go.mod` files, and `.ImportPath` follows the nearest `go.mod`.
- Type-aware scans with `typeCheck: true`, which load types with the Go toolchain, offline and for one platform, `linux/amd64` unless `platform` sets another. `implements` selects every type that implements an interface, with `.Pointer` when only its pointer does, and `firstParamType` and the `values` `type` filter compare types instead of spelling. A new diagnostic code `type-check` reports code that does not type check and missing dependencies.
- File scans, with a `files` block that lists files that are not Go source, such as SQL migrations, by a `name` pattern. With `each`, an output can render one file per listed file.
- Template function `sortBy`, which sorts items by a string or number field, such as `sortBy "Iota" .Colors` for declaration order.

### Changed

- Wildcards in scan directories no longer descend into a directory with its own `adder.yaml` or `go.mod`; name it outright to scan it.
- `adder explain <file>` uses the `adder.yaml` nearest the file rather than the current directory, and names the file as it was given.
- `derive`, `check`, and `list` reject arguments that are not directories, with a clearer message than before.

## [0.3.0] - 2026-10-30

### Added

- Scans skip files that no platform can build, such as `//go:build ignore`, evaluated the same way on every machine, and `buildTags` on a scan sets custom tags such as `integration`.
- Template functions `join`, `trimPrefix`, `trimSuffix`, `replace`, `quote`, `indent`, `nindent`, and `plural`, with Sprout's names and argument order, and `groupBy`.
- `adder new` asks for missing required inputs when run in a terminal, listing allowed values and asking again until an answer is valid.
- Generator `tests` and `adder test`, which compares the files a generator creates from sample inputs with golden files, and `adder test --update` to rewrite them. A new diagnostic code `golden-mismatch` reports failures.
- `--quiet` prints only problems, and `--verbose` also prints unchanged files and how many items each scan found.
- Statuses and problem codes are in color in a terminal, unless `NO_COLOR` is set or `TERM` is `dumb`.
- Shell completion for generator names, their flags, input values from `values` and `valuesFrom`, and generated files for `adder explain`, through `adder completion`.
- `adder list` shows the generators and derived outputs, `adder describe <generator>` shows a generator's inputs with the values its scans allow now and, with `--json`, a JSON Schema of its inputs, and `adder explain <file>` shows which output and template produced a file and whether it is up to date.
- `each` on an output renders it once per scan item, as `.Item`, with a templated path. Files it generated that it no longer produces are removed, and `adder check` reports them as stale. Changes gain a `removed` status.
- Generated-file headers for other languages, in each one's comment syntax: YAML, TOML, shell, Python, TypeScript, SQL, CSS, Markdown, HTML, and more.
- `--json` on every command prints the result and any problems as one JSON object on standard output, with a `usage` code for invocation mistakes. Exit codes are unchanged.

### Changed

- Outputs in languages other than Go now get adder's generated-code header.
- adder's header is added even when a template writes another tool's generated-code comment, which used to suppress it. Only adder's own header is kept as is.
- `adder derive` no longer replaces a file without adder's header with an output that has one, and reports `file-exists` instead. Non-Go outputs written by earlier versions have no header, so delete them once and run `adder derive` to regenerate them.
- `Item` is reserved and can no longer be used as a scan name.
- `adder derive` no longer lists unchanged outputs unless `--verbose` is given, so a run that changes nothing prints nothing.
- `-v` is now `--verbose`. Use `--version` to print the version.
- Scans skip files that no platform can build, such as `//go:build ignore` or a custom tag `buildTags` does not list, so derived outputs that included them change.
- adder refuses to write or remove a file through a symbolic link, with an `unsafe-path` problem.
- Scans no longer follow a symbolic link named directly in `dirs`, matching how wildcards already skipped them.

### Fixed

- A Go output whose template wrote another tool's generated-code comment was scanned on the next run, so it could feed back into its own scans.

- An unused import on the same line as the end of a block comment is removed without breaking the comment.

## [0.2.0] - 2026-10-29

### Added

- Directory patterns with `**`, which skip `testdata`, `vendor`, and hidden directories like `./...`, and `exclude` patterns.
- Function filters `kind`, `receiver`, and `firstParamType`, which matches the first parameter's type through the file's imports.
- Scan item fields `.Kind`, `.ImportPath`, `.Line`, `.Doc`, `.Signature`, `.Params`, and `.Results`.
- `//adder:` directives in doc comments, a `directive` filter, and `.Directives` on items.
- Type declaration scans with a `types` block, filtering by export, name, kind, and directive, with `.Fields` for structs.
- Top-level `vars`, available to every template as `.Vars`, and `partials`, templates whose `{{ define }}` blocks every template can use.
- `adder new <generator>` creates files from generators declared in `adder.yaml`, with string and bool inputs as flags, from a JSON file, or from standard input. Inputs can be restricted with `pattern`, `values`, `valuesFrom`, and `unique`; files have templated paths, `when` and `unless` conditions, and an `ifExists` policy that never overwrites. Derived outputs are updated in the same run, and `--dry-run` previews everything.
- `adder init` writes a starter `adder.yaml` with commented examples, never overwriting an existing one.
- Commands find `adder.yaml` by walking up from the current directory, like `go` finds `go.mod`, so they work from anywhere in the repository.
- Go files adder renders, both derived outputs and files from generators, lose the imports they never use, so templates can import everything a file might need.
- `--diff` on `adder new`, `adder derive`, and `adder check` shows every change as a unified diff that `git apply` accepts, using a copy of the Go toolchain's diff code. On `new` and `derive` it implies `--dry-run`.
- Diagnostic codes `unknown-generator`, `missing-input`, `invalid-input`, and `file-exists`, and a `skipped` change status.
- Hints for missing fields in nested maps, such as `.Vars.Licence`, and for undefined templates, listing the names that exist.

### Changed

- `Vars` and `Inputs` are reserved and can no longer be used as scan names.
- `--root` defaults to the directory holding `adder.yaml` instead of the current directory, and `--config` alone makes its directory the root.
- Usage errors point at the help of the command that was run, such as `adder derive --help`.
- Files that did not exist when the plan was made are created exclusively, so one that appears before writing is never overwritten.

## [0.1.0] - 2026-10-29

### Added

- `adder derive` rebuilds files from scans of the repository's Go source, with `--dry-run` to preview and `--out` to write elsewhere.
- `adder check` reports derived files that are out of date and exits with status 3, for CI.
- Scans by directory glob, exported names, first parameter name, and name pattern, with optional test files. Files adder generated are skipped, so outputs never feed back into scans.
- Scan items with `.Name`, `.Receiver`, `.Package`, `.Dir`, and `.File`, plus `.Module` from `go.mod`.
- Go outputs formatted with `gofmt` and marked with Go's standard generated-code header.
- Template functions with Sprout's names, including case conversion that understands acronyms and Go-style variants such as `toGoPascalCase`.
- Diagnostics with stable codes, file positions, and hints; every problem is reported at once.
- Exit codes: 0 for success, 1 for problems, 2 for usage mistakes, and 3 for stale outputs.

[Unreleased]: https://github.com/TyphonHill/adder/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/TyphonHill/adder/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/TyphonHill/adder/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/TyphonHill/adder/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/TyphonHill/adder/releases/tag/v0.1.0
