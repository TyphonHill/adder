# Changelog

All notable changes to adder are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and adder uses [semantic versioning](https://semver.org/). Before v1.0.0, a minor release may include breaking changes; they are listed under **Changed** or **Removed**.

## [Unreleased]

### Added

- `examples/`: ten small repositories, each showing one thing adder does and named after it, from `registry-from-functions` and `enum-strings` to `scaffold-with-generator`, `shared-pack`, `custom-scanner`, and `monorepo`. Each README says what the example shows and how, and adder's tests check and build every example with adder from the same commit.
- `docs/`: the reference, moved from the README into one file per topic: getting started, configuration, templates, extending adder, and commands. `docs/migrating.md` explains how to move to adder from a generator of your own.
- `docs/compatibility.md`: what v1.0 promises and what it does not, and the deprecation and Go version policies. Tests pin the lists it freezes: the file types that get a header, the initialisms, the template functions, and the reserved template fields.
- Deprecation warnings: adder warns, at its place, wherever something deprecated is used, and says what to use instead, without failing the command or changing its exit code. Generators, inputs, and pack options take `deprecated`, and `adder.Pack` takes `Deprecated` for a pack's functions and scanners, so repositories and packs can retire what they define the same way. `--help`, `adder list`, and `adder describe` mark what is deprecated. A deprecated input or option needs a default.
- JSON Schemas for `adder.yaml` and `pack.yaml`, in `schema/`, for completion and checking in editors. `adder init` and the examples point editors at them with a `yaml-language-server` line.
- The roadmap's launch milestone: what adder needs before it is published as a public tool, after v1.0.

### Changed

- The README is a short introduction: what adder does, one example, its features, and links to `docs/` and `examples/`.
- When `file-exists` blocks a file that another tool generated, its hint says so, and that deleting the file once lets adder generate it.
- The `adder.yaml` that `adder init` writes links to `docs/configuration.md`.
- Every JSON report has a `warnings` list beside `problems`, empty when there are none.

These changes settle names before v1.0 freezes them. Configurations that use an old name fail to load with `unknown-field` or `invalid-value`, naming the new one.

- `funcs.firstParam` is now `funcs.firstParamName`, to tell it apart from `firstParamType`.
- The type kind `type` is now `other`, in `types.kind` and in `.Kind`.
- A pack use renames its generators with `rename`, which was `generators`.
- `.Module` is always present, empty when the repository has no `go.mod`, as `.ImportPath` already was.
- `adder explain --json` names the header's text `source`, which was `template`, since it names the pack and the use too and `output.template` is the template's path.
- The Go case functions use staticcheck's ST1003 list of initialisms instead of golint's, so `toGoPascalCase "user_db"` is `UserDB`. It adds `DB`, `GID`, `SIP`, `RTP`, `AMQP`, and `TS`, and drops `LHS` and `RHS`. The list is frozen from v1.0.

### Fixed

- Positions in templates were one column to the left of the problem, since text/template counts columns from 0. They now count from 1, as every other position does.

## [0.6.0] - 2026-10-30

### Changed

- The module path is now `typhonhill.dev/adder`, and the repository lives at `github.com/typhonhilldev/adder`. Install adder with `go get -tool typhonhill.dev/adder/cmd/adder`, and change imports of `github.com/TyphonHill/adder` to `typhonhill.dev/adder`. Versions up to v0.5.1 stay available under the old path, which is deprecated, and are retracted under the new one, where they cannot be used.

## [0.5.1] - 2026-10-30

### Deprecated

- The module path `github.com/TyphonHill/adder`. adder moved to `typhonhill.dev/adder`, where new versions are released: install it with `go get -tool typhonhill.dev/adder/cmd/adder`, and change imports of `github.com/TyphonHill/adder` to `typhonhill.dev/adder`. Nothing else changes.

## [0.5.0] - 2026-10-30

### Added

- Packs: a Go package can share derive outputs, generators, and their templates, embedded with a `pack.yaml` that declares typed `options`. A repository uses a pack under `packs` in `adder.yaml`, once or several times, giving its options under `with`, and builds its own adder with the pack. Problems in a pack's files carry the pack's name, as `pack` in JSON, and a new diagnostic code `unknown-pack` reports a configuration that uses a pack adder was not built with. A use's generators become `adder new` commands, which a use can rename, and a use tests them with golden files in the repository. The repository's own outputs and generator files can render a pack's template with `pack`.
- Custom template functions: a repository builds its own adder from a small `main` package that calls `adder.Main` with an `adder.Pack` of functions, and runs it with `go tool adder`. The root package `github.com/TyphonHill/adder` is the first public Go API, experimental until v1.0. A new diagnostic code `invalid-pack` reports a pack that cannot be registered, such as a function named like a built-in one.
- Custom scanners: a pack's `Scanners` are Go functions that a scan runs with `scanner: {name, with}`, reading the repository and returning structs or maps, which templates and generator inputs read by field name. A returned `adder.Problem` is reported at its file. New diagnostic codes `scanner` and `unknown-scanner` report a failing scanner and one adder was not built with.

### Changed

- Function names in output and generator paths are checked when the path renders rather than when `adder.yaml` loads, so paths can use a repository's own functions.
- `Options` is reserved and can no longer be used as a scan name.
- A scan directory containing `{{` is rejected, since directories are not templates.
- `fieldValues`, `sortBy`, and `groupBy` also read the keys of maps, as custom scanners may return.

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

[Unreleased]: https://github.com/typhonhilldev/adder/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/typhonhilldev/adder/compare/v0.5.1...v0.6.0
[0.5.1]: https://github.com/typhonhilldev/adder/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/typhonhilldev/adder/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/typhonhilldev/adder/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/typhonhilldev/adder/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/typhonhilldev/adder/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/typhonhilldev/adder/releases/tag/v0.1.0
