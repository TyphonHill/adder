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

Status: ✅ available · 🔜 planned for the milestone shown · 💡 idea, not a commitment.

### Configuration

| Feature | Status |
| --- | --- |
| `adder.yaml` with a schema `version` | ✅ |
| Strict decoding: unknown keys and invalid patterns fail at load time | ✅ |
| Every validation problem reported at once, with its line and column | ✅ |
| Paths are slash-separated, relative, confined to their root, and cleaned on load | ✅ |
| Find `adder.yaml` by walking up from the current directory, like `go` finds `go.mod` (one file name, [design 12](docs/design.md#12-one-configuration-file-at-the-root)) | 🔜 v0.2 |
| JSON Schema for `adder.yaml`, published to SchemaStore for editor completion and validation | 🔜 v0.2 |
| `adder validate`: check the configuration and templates without writing anything | 🔜 v0.2 |
| Top-level `vars` shared by every template, such as a license header | ✅ |
| Multiple configurations in one repository, for monorepos and nested modules | 🔜 v0.4 |
| `adder migrate`: upgrade a configuration to a newer schema version | 🔜 v1.0 |

### Scanning

| Feature | Status |
| --- | --- |
| Directory globs, exported names, first parameter name, name pattern | ✅ |
| Optional `_test.go` files | ✅ |
| Items with `.Name`, `.Receiver`, `.Package` (from the `package` clause), `.Dir`, `.File` | ✅ |
| `.ImportPath` on every item, so templates stop assembling import paths by hand ([design 20](docs/design.md#20-richer-scans-without-type-checking-planned-v02)) | ✅ |
| Skip files adder generated, so outputs never feed back into scans; other generators' files are still scanned | ✅ |
| `exclude` patterns and recursive `**` globs that skip `testdata`, `vendor`, and hidden directories like `./...` | ✅ |
| Skip files excluded by build constraints such as `//go:build ignore` | 🔜 v0.2 |
| Match on `kind` (functions or methods) and `receiver` | ✅ |
| `firstParamType`: match the first parameter's type as written, resolved through the file's imports ([design 20](docs/design.md#20-richer-scans-without-type-checking-planned-v02)) | ✅ |
| `.Kind`, `.Doc`, `.Line`, `.Signature`, `.Params`, and `.Results` on every item | ✅ |
| Comment directives in Go's directive syntax, such as `//adder:register name=s3`: a `directive` filter and `.Directives` on items ([design 16](docs/design.md#16-comment-directives-use-gos-directive-syntax-planned-v02)) | ✅ |
| Scan type declarations with a `types` block: structs, interfaces, and other types, with `.Fields` for structs | ✅ |
| Type-aware matching, opted into per scan with `typeCheck: true`: types that implement an interface, and type matches that survive any import alias ([design 17](docs/design.md#17-type-aware-scans-are-an-explicit-opt-in-planned-v04)) | 🔜 v0.4 |
| Scan constants and variables, for enums and lookup tables | 🔜 v0.4 |
| File scans: list non-Go files such as SQL migrations or JSON fixtures | 🔜 v0.4 |
| Parallel parsing and caching for very large repositories | 💡 |

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
| Fix imports automatically, goimports-style, so templates need not manage them | 🔜 v0.2 |
| Shared template partials, such as `templates/partials/license.tmpl` | ✅ |
| More functions, using Sprout's names where they exist: `join`, `trimPrefix`, `trimSuffix`, `replace`, `quote`, `indent`, `pluralize`, `singularize`, `groupBy` | 🔜 v0.2 |
| One output per item, with a templated path, for example a file per interface | 🔜 v0.3 |
| Remove outputs that are no longer produced, found by their header within each output's path pattern ([design 15](docs/design.md#15-stale-outputs-are-found-by-header-within-each-outputs-own-path-planned-v03)) | 🔜 v0.3 |
| Generated-file headers for other languages, using their comment syntax | 🔜 v0.3 |
| `--stdout` to print a single output instead of writing it | 💡 |

### Scaffolding: `adder new`

| Feature | Status |
| --- | --- |
| Generators that create new files from templates, as `adder new <generator>` ([design 19](docs/design.md#19-scaffolding-with-adder-new-planned-v02)) | ✅ |
| String and bool inputs with `help`, `default`, `pattern`, and `values`; a string without a default is required | ✅ |
| `valuesFrom`: choices taken from a scan, such as "package must be an existing directory under `drivers/`" | ✅ |
| `unique`: reject values a scan already has, such as "no driver with this name already exists" | ✅ |
| Templated file paths, and files included with `when` or `unless` on a bool input | ✅ |
| Existing-file policy: `fail` or `skip`, never overwrite | ✅ |
| `--dry-run` with the planned changes | ✅ |
| `--diff` for a unified diff of each change | 🔜 v0.2 |
| New files and the derived outputs they change computed as one plan, so registries update in the same run | ✅ |
| A message after generating, with next steps for the person who ran it | ✅ |
| Inputs from flags, from a JSON file, or from standard input | ✅ |
| Interactive prompts for missing inputs when running in a terminal | 🔜 v0.3 |
| Generator tests: render a generator with sample inputs and compare to golden files | 🔜 v0.3 |
| Composing generators, where one generator runs another | 💡 |
| Inserting code into existing files through the syntax tree, for cases deriving cannot cover | 💡 |

### Discovery and agents

| Feature | Status |
| --- | --- |
| `adder list`: every generator and derived output | 🔜 v0.3 |
| `adder describe <generator>` with a JSON Schema of its inputs | 🔜 v0.3 |
| Structured diagnostics with stable codes, positions, and hints | ✅ |
| JSON output for every command | 🔜 v0.3 |
| `adder explain <file>`: which configuration and template produced a generated file | 🔜 v0.3 |
| A guide and a ready-made `AGENTS.md` section for repositories that use coding agents | 🔜 v0.3 |
| `adder mcp`: an MCP server exposing generators as tools, read-only unless asked to write | 💡 |

### Command line

| Feature | Status |
| --- | --- |
| `--root` and `--config` | ✅ |
| `--version` from build information | ✅ |
| `adder derive --dry-run` | ✅ |
| Documented exit codes for success, error, usage, and stale outputs | ✅ |
| `adder check`: exit with status 3 when derived files are stale, for CI and repositories that commit them | ✅ |
| `adder init`: create a starter `adder.yaml` and example template | 🔜 v0.2 |
| Shell completion for commands, generator names, and inputs | 🔜 v0.2 |
| `--quiet`, `--verbose`, and `NO_COLOR` support | 🔜 v0.2 |
| `adder derive --watch` for a fast edit loop | 💡 |
| `adder graph`: a Mermaid diagram of which outputs depend on which scans and templates | 💡 |

### Integrations

| Feature | Status |
| --- | --- |
| `go get -tool` and `go tool adder` | ✅ |
| Documented `//go:generate go tool adder derive` usage | 🔜 v0.2 |
| A pre-commit hook definition (`.pre-commit-hooks.yaml`) | 🔜 v0.2 |
| A GitHub Action that runs `adder check` on pull requests | 🔜 v0.3 |
| A GitLab CI example | 🔜 v0.3 |
| Generator packs: generators shared across repositories as versioned Go modules | 🔜 v0.5 |

### Library API

| Feature | Status |
| --- | --- |
| Internal pipeline built on plans, `fs.FS`, and `context.Context`, ready to expose | ✅ |
| A public facade in the root package, shipped as experimental once a real consumer needs it ([design 18](docs/design.md#18-the-library-api-arrives-when-it-has-a-real-consumer-planned)) | 🔜 v0.5 |
| Register custom template functions and scanners from Go code | 🔜 v0.5 |
| Compatibility guarantee for the library API | 🔜 v1.0 |

### Quality and security

| Feature | Status |
| --- | --- |
| Table-driven and golden-file tests, race detector, golangci-lint, govulncheck in CI | ✅ |
| Tests on the two supported Go releases | ✅ |
| CI on Linux, macOS, and Windows, since paths and line endings differ | ✅ |
| GitHub Actions pinned to commit SHAs and updated by Dependabot | ✅ |
| Repository hardening: branch and tag rulesets, required checks, secret scanning, private vulnerability reporting | 🔜 v0.1 |
| A test that keeps README examples in sync with the test fixtures, once the documentation site has many examples | 💡 |
| End-to-end CLI tests with `testscript` | 🔜 v0.2 |
| Fuzz tests for configuration parsing and header detection | 🔜 v0.2 |
| Benchmarks on large repositories with a performance budget | 🔜 v0.3 |
| A symlink policy for scans and outputs | 🔜 v0.3 |
| OpenSSF Scorecard and CodeQL | 🔜 v0.3 |

### Distribution

| Feature | Status |
| --- | --- |
| Release automation with GoReleaser and reproducible builds (`-trimpath`), for published binaries | 🔜 v0.2 |
| Signed checksums, SBOMs, and build provenance | 🔜 v0.2 |
| Homebrew tap | 🔜 v0.2 |
| A container image for CI systems without Go | 💡 |

### Documentation and community

| Feature | Status |
| --- | --- |
| README with a tested quick start | ✅ |
| `CHANGELOG.md` following Keep a Changelog, with semantic versioning | ✅ |
| `CONTRIBUTING.md` with build, test, and code conventions | ✅ |
| `SECURITY.md` with private vulnerability reporting | ✅ |
| `CODE_OF_CONDUCT.md` | 🔜 v1.0 |
| Issue and pull request templates | 🔜 v0.2 |
| Runnable `examples/` directory, tested in CI | 🔜 v0.2 |
| Documentation site: guides, configuration reference generated from the schema, function reference | 🔜 v0.3 |
| Recipes: driver and plugin registries, HTTP routes, CLI commands, gRPC wiring, workflow engines | 🔜 v0.3 |
| Comparison page: adder versus `go generate`, stencil, plop, hygen, and hand-rolled generators | 🔜 v0.3 |
| Migration guide from a hand-rolled generator | 🔜 v0.3 |
| Support and deprecation policy for configuration fields and Go versions | 🔜 v1.0 |
| Launch: announcement post, Gophers Slack, r/golang | 🔜 v0.3 |

## Milestones

| Milestone | Theme | Done when |
| --- | --- | --- |
| **v0.1 derive** | Derived files that are safe to rely on in CI | `adder derive` and `adder check` pass end to end on Linux, macOS, and Windows with both supported Go releases, and a hand-rolled registry generator can be reproduced from configuration alone. |
| **v0.2 new** | Scaffolding with validated inputs | A new component can be added with one `adder new` command, and its registries update automatically. |
| **v0.3 discover** | People and agents can find and drive generators without reading the configuration | An agent can list generators, read their input schema, preview, and run one, using only adder's JSON output. |
| **v0.4 types** | Scans that understand Go types, not only names | Registries can be built from "every type that implements this interface". |
| **v0.5 packs** | Reuse across repositories, plus a public library API | Two unrelated repositories share a generator pack. |
| **v1.0** | Stability | The configuration schema and library API carry compatibility guarantees. |

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
