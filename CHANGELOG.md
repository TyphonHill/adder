# Changelog

All notable changes to adder are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and adder uses [semantic versioning](https://semver.org/). Before v1.0.0, a minor release may include breaking changes; they are listed under **Changed** or **Removed**.

## [Unreleased]

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

[Unreleased]: https://github.com/TyphonHill/adder/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/TyphonHill/adder/releases/tag/v0.1.0
