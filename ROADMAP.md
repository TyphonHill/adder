# Roadmap

## North star

**The conventions a Go team already follows, declared once and kept correct automatically, by people and coding agents alike.**

Every codebase grows patterns: a registry that lists every driver, a handler that must be wired into a router, a new job that needs the same four files. Today those patterns live in someone's head, a wiki page, or a hand-rolled generator. adder makes them a small, reviewable part of the repository that anyone, or any agent, can run.

## Principles

These keep adder small enough to learn in an afternoon and maintain for years. A feature that breaks one of them needs a very good reason.

1. **Plain Go templates.** No new template language and no YAML programming. If a feature needs conditionals or loops in `adder.yaml`, it belongs in the template or in Go code instead.
2. **Declarative first, Go as the escape hatch.** The configuration covers the common cases. Anything more complex is better served by calling adder as a library than by growing the configuration.
3. **No surprises.** Output stays inside the root, nothing is written unless everything renders, unchanged files are left alone, and every change can be previewed first.
4. **Generated or owned, never both.** Derived files are regenerated and marked `DO NOT EDIT`. Scaffolded files belong to people once created, and adder never rewrites them.
5. **Prefer deriving over editing.** Wiring new code in by regenerating a file from the source is safer than patching a file people edit. Editing existing files is a last resort.
6. **Deterministic.** The same repository and configuration produce byte-identical output on every machine and operating system. No timestamps, no map ordering, no environment leaks.
7. **Safe on untrusted repositories.** Running adder never executes code from the repository or its configuration: no shell hooks, no plugins loaded at runtime, no network access.
8. **Machine-readable by default.** Every command that reports something can report it as JSON, with stable exit codes, so scripts and agents can drive adder as easily as people.
9. **Fast and self-contained.** A single binary that works offline, installs with `go get -tool`, and finishes in well under a second on large repositories.
10. **A good neighbor.** adder works alongside `go generate`, pre-commit, CI, and other generators rather than replacing them.

## Features

Status: ✅ available · 🔜 planned for the milestone shown: v1.0, or the launch, when adder is published as a public tool · 💡 idea, not a commitment.

### Configuration

| Feature | Status |
| --- | --- |
| `adder.yaml` with a schema `version` | ✅ |
| Strict decoding: unknown keys and invalid patterns fail at load time | ✅ |
| Every validation problem reported at once, with its line and column | ✅ |
| Paths are slash-separated, relative, confined to their root, and cleaned on load | ✅ |
| Find `adder.yaml` by walking up from the current directory, like `go` finds `go.mod` (one file name, [design 12](docs/design.md#12-one-configuration-file-at-the-root)) | ✅ |
| JSON Schemas for `adder.yaml` and `pack.yaml`, for editor completion and validation ([design 31](docs/design.md#31-checking-the-configuration-planned-v10)) | ✅ |
| The schema listed in SchemaStore, so editors find it without a modeline | 🔜 launch |
| Top-level `vars` shared by every template, such as a license header | ✅ |
| Multiple configurations in one repository, for monorepos and nested modules ([design 40](docs/design.md#40-multiple-configurations)) | ✅ |
| A `file-exists` hint that names another tool's generated file, for moving to adder ([design 44](docs/design.md#44-moving-from-a-hand-rolled-generator-planned-v10)) | ✅ |
| `adder migrate`: upgrade a configuration to a newer schema version, with the first schema version that needs it ([design 43](docs/design.md#43-reaching-v10-planned-v10)) | 💡 |

### Scanning

| Feature | Status |
| --- | --- |
| Directory globs, exported names, first parameter name, name pattern | ✅ |
| Optional `_test.go` files | ✅ |
| Items with `.Name`, `.Receiver`, `.Package` (from the `package` clause), `.Dir`, `.File` | ✅ |
| `.ImportPath` on every item, so templates stop assembling import paths by hand ([design 20](docs/design.md#20-richer-scans-without-type-checking)) | ✅ |
| Skip files adder generated, so outputs never feed back into scans; other generators' files are still scanned | ✅ |
| `exclude` patterns and recursive `**` globs that skip `testdata`, `vendor`, and hidden directories like `./...` | ✅ |
| Skip files excluded by build constraints such as `//go:build ignore` ([design 27](docs/design.md#27-build-constraints-in-scans)) | ✅ |
| Match on `kind` (functions or methods) and `receiver` | ✅ |
| `firstParamType`: match the first parameter's type as written, resolved through the file's imports ([design 20](docs/design.md#20-richer-scans-without-type-checking)) | ✅ |
| `.Kind`, `.Doc`, `.Line`, `.Signature`, `.Params`, and `.Results` on every item | ✅ |
| Comment directives in Go's directive syntax, such as `//adder:register name=s3`: a `directive` filter and `.Directives` on items ([design 16](docs/design.md#16-comment-directives-use-gos-directive-syntax)) | ✅ |
| Scan type declarations with a `types` block: structs, interfaces, and other types, with `.Fields` for structs | ✅ |
| Type-aware matching, opted into per scan with `typeCheck: true`: types that implement an interface, and type matches that survive any import alias ([design 17](docs/design.md#17-type-aware-scans-are-an-explicit-opt-in), [37](docs/design.md#37-loading-types)) | ✅ |
| Scan constants and variables, for enums and lookup tables ([design 38](docs/design.md#38-constant-and-variable-scans)) | ✅ |
| File scans: list non-Go files such as SQL migrations or JSON fixtures ([design 39](docs/design.md#39-file-scans)) | ✅ |
| Parallel parsing and caching for very large repositories | 🔜 launch |

### Rendering and outputs

| Feature | Status |
| --- | --- |
| Go `text/template` with a curated, deterministic function set using Sprout's names, implemented without dependencies | ✅ |
| Case conversion that understands acronyms, with Go-style variants (`toGoPascalCase "user_id"` is `UserID`) | ✅ |
| Missing template fields are errors, not empty strings | ✅ |
| Go outputs formatted with `gofmt` | ✅ |
| Standard `// Code generated ... DO NOT EDIT.` header added to Go outputs that lack one | ✅ |
| `.Module` from `go.mod` | ✅ |
| Nothing written unless every output renders; unchanged files untouched | ✅ |
| Every rendering problem reported together, not only the first | ✅ |
| Template line endings normalized, so output is identical on every operating system | ✅ |
| Outputs confined to the root | ✅ |
| `--out` to write somewhere else for previews | ✅ |
| Remove unused imports from Go output, so templates need not guard each one ([design 21](docs/design.md#21-unused-imports-are-removed-missing-ones-are-not-added)) | ✅ |
| Shared template partials, such as `templates/partials/license.tmpl` | ✅ |
| More functions, using Sprout's names where they exist: `join`, `trimPrefix`, `trimSuffix`, `replace`, `quote`, `indent`, `nindent`, `plural`, `groupBy` ([design 28](docs/design.md#28-more-template-functions)) | ✅ |
| One output per item, with a templated path, for example a file per interface ([design 24](docs/design.md#24-one-output-per-item)) | ✅ |
| Remove outputs that are no longer produced, found by their header within each output's path pattern ([design 15](docs/design.md#15-stale-outputs-are-found-by-header-within-each-outputs-own-path)) | ✅ |
| Generated-file headers for other languages, using their comment syntax ([design 25](docs/design.md#25-generated-file-headers-for-other-languages)) | ✅ |
| `--stdout` to print a single output instead of writing it | 🔜 launch |

### Scaffolding: `adder new`

| Feature | Status |
| --- | --- |
| Generators that create new files from templates, as `adder new <generator>` ([design 19](docs/design.md#19-scaffolding-with-adder-new)) | ✅ |
| String and bool inputs with `help`, `default`, `pattern`, and `values`; a string without a default is required | ✅ |
| `valuesFrom`: choices taken from a scan, such as "package must be an existing directory under `drivers/`" | ✅ |
| `unique`: reject values a scan already has, such as "no driver with this name already exists" | ✅ |
| Templated file paths, and files included with `when` or `unless` on a bool input | ✅ |
| Existing-file policy: `fail` or `skip`, never overwrite | ✅ |
| `--dry-run` with the planned changes | ✅ |
| `--diff` for a unified diff of each change | ✅ |
| New files and the derived outputs they change computed as one plan, so registries update in the same run | ✅ |
| A message after generating, with next steps for the person who ran it | ✅ |
| Inputs from flags, from a JSON file, or from standard input | ✅ |
| Interactive prompts for missing inputs when running in a terminal ([design 29](docs/design.md#29-prompts-for-missing-inputs)) | ✅ |
| Generator tests: render a generator with sample inputs and compare to golden files ([design 30](docs/design.md#30-generator-tests)) | ✅ |
| Composing generators, where one generator runs another | 🔜 launch |

### Discovery and agents

| Feature | Status |
| --- | --- |
| `adder list`: every generator and derived output ([design 23](docs/design.md#23-discovering-generators-and-outputs)) | ✅ |
| `adder describe <generator>` with a JSON Schema of its inputs ([design 23](docs/design.md#23-discovering-generators-and-outputs)) | ✅ |
| Structured diagnostics with stable codes, positions, and hints | ✅ |
| JSON output for every command ([design 22](docs/design.md#22-json-output)) | ✅ |
| `adder explain <file>`: which configuration and template produced a generated file ([design 23](docs/design.md#23-discovering-generators-and-outputs)) | ✅ |
| A guide and a ready-made `AGENTS.md` section for repositories that use coding agents ([design 34](docs/design.md#34-integrations-and-coding-agents-planned-v10-and-launch)) | 🔜 v1.0 |
| `adder mcp`: an MCP server exposing generators as tools, read-only unless asked to write | 🔜 launch |

### Command line

| Feature | Status |
| --- | --- |
| `--root` and `--config` | ✅ |
| `--version` from build information | ✅ |
| `adder derive --dry-run` | ✅ |
| Documented exit codes for success, error, usage, and stale outputs | ✅ |
| `adder check`: exit with status 3 when derived files are stale, for CI and repositories that commit them | ✅ |
| `adder init`: create a starter `adder.yaml` with commented examples of each section | ✅ |
| Shell completion for commands, generator names, and inputs ([design 32](docs/design.md#32-terminal-output)) | ✅ |
| `--quiet`, `--verbose`, and `NO_COLOR` support ([design 32](docs/design.md#32-terminal-output)) | ✅ |
| `adder derive --watch` for a fast edit loop | 🔜 launch |
| `adder graph`: a Mermaid diagram of which outputs depend on which scans and templates | 🔜 launch |

### Integrations

| Feature | Status |
| --- | --- |
| `go get -tool` and `go tool adder` | ✅ |
| Documented `//go:generate go tool adder derive` usage ([design 34](docs/design.md#34-integrations-and-coding-agents-planned-v10-and-launch)) | 🔜 v1.0 |
| A pre-commit hook definition (`.pre-commit-hooks.yaml`) ([design 34](docs/design.md#34-integrations-and-coding-agents-planned-v10-and-launch)) | 🔜 v1.0 |
| A GitHub Action that runs `adder check` on pull requests ([design 34](docs/design.md#34-integrations-and-coding-agents-planned-v10-and-launch)) | 🔜 launch |
| Documented CI jobs for GitLab CI and GitHub Actions, running `adder check` ([design 34](docs/design.md#34-integrations-and-coding-agents-planned-v10-and-launch)) | 🔜 v1.0 |
| Generator packs: generators, derive outputs, and templates shared across repositories as versioned Go packages ([design 41](docs/design.md#41-generator-packs)) | ✅ |

### Library API

| Feature | Status |
| --- | --- |
| Internal pipeline built on plans, `fs.FS`, and `context.Context`, ready to expose | ✅ |
| A public facade in the root package, experimental until v1.0: `adder.Main` and `adder.Pack` for a repository's own build ([design 18](docs/design.md#18-the-library-api-starts-with-packs-and-extensions)) | ✅ |
| Register custom template functions and scanners from Go code ([design 42](docs/design.md#42-custom-template-functions-and-scanners)) | ✅ |
| Run adder from Go, with plans and diagnostics as Go values, for the MCP server and editor integrations ([design 18](docs/design.md#18-the-library-api-starts-with-packs-and-extensions)) | 🔜 launch |
| Compatibility guarantee for the library API ([design 43](docs/design.md#43-reaching-v10-planned-v10)) | ✅ |

### Quality and security

| Feature | Status |
| --- | --- |
| Table-driven and golden-file tests, race detector, golangci-lint, govulncheck in CI | ✅ |
| Tests on the two supported Go releases | ✅ |
| CI on Linux, macOS, and Windows, since paths and line endings differ | ✅ |
| GitHub Actions pinned to commit SHAs and updated by Dependabot | ✅ |
| Secret scanning with push protection, and private vulnerability reporting | ✅ |
| Repository hardening: branch and tag rulesets with required checks and reviews, and immutable release tags | ✅ |
| A test that keeps README examples in sync with the test fixtures, once the documentation site has many examples | 💡 |
| End-to-end CLI tests with `testscript` ([design 36](docs/design.md#36-testing-adder-itself)) | ✅ |
| A review of everything v1.0 freezes: schemas, template data and functions, the header, codes, JSON, and the Go API ([design 43](docs/design.md#43-reaching-v10-planned-v10)) | ✅ |
| A trial on a production codebase with a hand-rolled generator, before the schema freezes ([design 43](docs/design.md#43-reaching-v10-planned-v10)) | ✅ |
| Fuzz tests for configuration parsing, header detection, directives, and import pruning ([design 36](docs/design.md#36-testing-adder-itself)) | ✅ |
| Benchmarks on large repositories with a performance budget ([design 36](docs/design.md#36-testing-adder-itself)) | ✅ |
| A symlink policy for scans and outputs ([design 26](docs/design.md#26-symlinks)) | ✅ |
| CodeQL | ✅ |
| OpenSSF Scorecard | 🔜 launch |

### Distribution

| Feature | Status |
| --- | --- |
| Release automation with GoReleaser and reproducible builds (`-trimpath`), for published binaries ([design 33](docs/design.md#33-releases-planned-launch)) | 🔜 launch |
| Signed checksums, SBOMs, and build provenance ([design 33](docs/design.md#33-releases-planned-launch)) | 🔜 launch |
| Homebrew tap ([design 33](docs/design.md#33-releases-planned-launch)) | 🔜 launch |
| A container image for CI systems without Go | 🔜 launch |

### Documentation and community

| Feature | Status |
| --- | --- |
| README with a tested quick start | ✅ |
| README rewritten to show what adder does, with the reference in `docs/`, one file per topic ([design 43](docs/design.md#43-reaching-v10-planned-v10)) | ✅ |
| `CHANGELOG.md` following Keep a Changelog, with semantic versioning | ✅ |
| `CONTRIBUTING.md` with build, test, and code conventions | ✅ |
| `SECURITY.md` with private vulnerability reporting | ✅ |
| `CODE_OF_CONDUCT.md`, with a way to report problems that publishes no one's address, at the launch | 🔜 launch |
| Issue and pull request templates | 🔜 launch |
| Runnable `examples/` directory, tested in CI: ten single-capability examples, covering every scan kind, `each`, generators, packs, custom scanners, and a monorepo ([design 43](docs/design.md#43-reaching-v10-planned-v10)) | ✅ |
| Documentation site: guides, configuration reference generated from the schema, function reference ([design 35](docs/design.md#35-documentation-and-examples-planned-launch)) | 🔜 launch |
| Recipes: driver and plugin registries, HTTP routes, CLI commands, gRPC wiring, workflow engines | 🔜 launch |
| Comparison page: adder versus `go generate`, stencil, plop, hygen, and hand-rolled generators | 🔜 launch |
| Migration guide from a hand-rolled generator, in `docs/migrating.md` ([design 44](docs/design.md#44-moving-from-a-hand-rolled-generator-planned-v10)) | ✅ |
| Support and deprecation policy for configuration fields and Go versions, in `docs/compatibility.md` ([design 43](docs/design.md#43-reaching-v10-planned-v10)) | ✅ |
| Deprecation warnings at each use, for adder's own settings, functions, fields, commands, and flags, and for what repositories and packs define ([design 45](docs/design.md#45-deprecation-warnings)) | ✅ |
| Launch: announcement post, Gophers Slack, r/golang | 🔜 launch |

## Milestones

| Milestone | Theme | Done when |
| --- | --- | --- |
| **v0.1 derive** | Derived files that are safe to rely on in CI | `adder derive` and `adder check` pass end to end on Linux, macOS, and Windows with both supported Go releases, and a hand-rolled registry generator can be reproduced from configuration alone. |
| **v0.2 new** | Scaffolding with validated inputs | A new component can be added with one `adder new` command, and its registries update automatically. |
| **v0.3 discover** | People and agents can find and drive generators without reading the configuration | An agent can list generators, read their input schema, preview, and run one, using only adder's JSON output. |
| **v0.4 types** | Scans that understand Go types, not only names | Registries can be built from "every type that implements this interface". |
| **v0.5 packs** | Reuse across repositories, plus a public library API | Two unrelated repositories share a generator pack. |
| **v1.0** | Stability | The configuration schema and library API carry compatibility guarantees. |
| **Launch** | Distribution, integrations, and documentation | Releases publish signed binaries and a Homebrew cask, editors, CI systems, and coding agents through MCP integrate with adder, a documentation site covers every feature, and adder is announced as a public tool. |

## Open questions

None at the moment. Settled decisions, including those for planned features, are recorded in [docs/design.md](docs/design.md).

## Non-goals

- **Bootstrapping new projects.** `gonew` and project templates already do this well. adder works inside repositories that already exist.
- **Keeping many repositories in sync with upstream templates.** Tools such as stencil and copier focus on that. adder does not merge upstream changes into files people have edited.
- **General refactoring.** Rewriting code across a codebase is the job of gopls, `gofmt -r`, and similar tools.
- **Replacing specialized generators.** protoc, sqlc, stringer, mockery, and oapi-codegen solve their problems better. adder sits alongside them.
- **A template language of its own.** Go's `text/template` and a curated set of functions are the whole story.
- **Running commands.** No shell hooks or post-generation scripts. Use `make`, `go generate`, or CI for that.
- **Scanning languages other than Go.** Outputs can be any text, but scans understand Go source and plain file lists only.

### Decided against

- **`adder validate`.** `adder derive --dry-run` already runs every scan and renders every output without writing anything, and exits 0 or 1, which is what a repository that does not commit its outputs needs from CI.
- **Short flags for generator inputs.** Long flags read better in scripts and documentation, and agents pass inputs as JSON ([design 44](docs/design.md#44-moving-from-a-hand-rolled-generator-planned-v10)).
- **Inserting code into files people edit.** Generated or owned, never both: adder never rewrites a file people own. Deriving a file from the code covers the same cases safely.
- **Adding missing imports.** A built-in table of the standard library would make output change between adder releases, and searching the module cache would make it depend on the machine ([design 21](docs/design.md#21-unused-imports-are-removed-missing-ones-are-not-added)).

## Ceiling

adder is worth building only if it stays simple, so these are the limits to watch.

- **Configuration creep.** The biggest risk is `adder.yaml` turning into a programming language, the way many YAML-driven tools have. The principles above are the defense: complexity goes into templates or Go, never into the configuration.
- **What declarations can express.** Name and parameter matching covers registries and wiring. Type-aware scans and comment directives raise that limit. Beyond them, the library API is the answer, not more matchers.
- **Audience.** Generators are a smaller category than CLI frameworks. Comparable tools in other ecosystems, such as plop and hygen, reach thousands of teams rather than most of a language community. The upside is coding agents: deterministic, discoverable generators give agents a way to follow a repository's conventions exactly, at no cost, every time. If that becomes common practice, adder's audience is every Go team that uses agents.
- **Agents writing boilerplate directly.** Agents can already write the code adder generates, so scaffolding alone is not enough. Derived files are the durable value. Agents forget to update registries; `adder derive` does not.

## Success

- After v1.0, a large production codebase runs entirely on adder in place of its hand-rolled generator.
- A new user goes from install to a working generator in under ten minutes.
- Teams outside the original one adopt it without needing help from the maintainers.
- Pull requests from people who are not maintainers make up a meaningful share of changes by v1.0.
