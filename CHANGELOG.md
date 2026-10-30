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
- Hints for missing fields in nested maps, such as `.Vars.Licence`, and for undefined templates, listing the names that exist.

### Changed

- `Vars` and `Inputs` are reserved and can no longer be used as scan names.

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
