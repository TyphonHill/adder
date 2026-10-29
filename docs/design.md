# Design

This document explains how adder is built and why. It records the decisions that shape the code, so that future features extend the design instead of working around it. The [roadmap](../ROADMAP.md) covers what adder will do; this covers how.

## Architecture

```
adder.yaml ──► config ──► scan ──► render ──► plan ──► apply
                │           │         │         │        │
            validated    items per  file      changes  files
            settings     scan       content   (no I/O) written
```

| Package | Responsibility |
| --- | --- |
| `internal/config` | Load `adder.yaml`, validate it, and record where every setting is declared. |
| `internal/scan` | Parse Go source and return the declarations that match a scan. |
| `internal/render` | Execute a template into file content and format Go output. |
| `internal/header` | Write and recognize the generated-code header on adder's Go outputs. |
| `internal/plan` | Describe file changes before they happen, and apply them. |
| `internal/derive` | Connect the steps above for `adder derive` and `adder check`. |
| `internal/scaffold` | Resolve a generator's inputs, plan its new files, and derive outputs over them for `adder new`. |
| `internal/textcase` | Case conversion, shared by template functions and input flag names. |
| `internal/diff` | Unified diffs for previews, copied from the Go toolchain. |
| `internal/diag` | Structured problems shared by every package. |
| `internal/cli` | Commands, flags, output, and exit codes. |
| `cmd/adder` | The binary: signal handling and build information only. |

Each step only depends on the steps before it. Commands are thin: they load configuration, call one pipeline, and print the result.

## Decisions

### 1. Plan before writing

Computing changes and making them are separate steps. `derive.Plan` returns a `plan.Plan` listing every output with its current and intended content, and does no writing. `Plan.Apply` writes the changes.

**Why:** `--dry-run`, `adder check`, diffs, JSON output, and editor integrations all need to know what *would* change. With one plan, each of them is a few lines on top of it rather than another copy of the pipeline.

**Consequences:** nothing is written unless every output renders. Unchanged files are never rewritten, so their modification times stay put.

### 2. File systems, not paths

The pipeline reads through `fs.FS`: the repository, the template directory, and the existing outputs are three separate file systems. The command line opens them with `os.Root`, which also confines every write to its root.

**Why:** tests run against in-memory file systems (`fstest.MapFS`), an editor integration can supply unsaved buffers, and no path in the configuration can reach outside its root.

**Consequences:** paths inside the pipeline are slash-separated `fs` paths. Operating system paths only appear at the edges, in the command line and in diagnostics.

### 3. Context everywhere

Every step that does I/O takes a `context.Context` and checks it between files.

**Why:** type-aware scans, watch mode, and editor integrations need cancellation. Adding a context parameter after the library API is public would be a breaking change.

### 4. Structured diagnostics

Every problem is a `diag.Diagnostic` with a stable code, a position, a message, and an optional hint. Several problems are reported together as a `diag.List`. The command line prints them as `file:line:column: message [code]`, and the same values serialize to JSON.

Positions follow two rules:

- A problem inside a file carries that file's path, joined to the directory the file system came from, so it opens from wherever adder ran.
- A problem with a declaration, such as a scan that matches nothing, has no file position of its own and is attributed to where it is declared in `adder.yaml`.

| Code | Meaning |
| --- | --- |
| `error` | A problem without a more specific code. |
| `io` | A file could not be read or written. |
| `yaml-syntax` | `adder.yaml` is not valid YAML. |
| `unknown-field` | `adder.yaml` has a key adder does not recognize. |
| `invalid-value` | A value has the wrong type or is malformed, such as an invalid pattern. |
| `missing-field` | A required setting is absent. |
| `unsupported-version` | The configuration declares a schema version this release does not understand. |
| `invalid-name` | A scan or var name is not a valid Go identifier. |
| `reserved-name` | A scan name collides with data adder provides: `Module`, `Vars`, or `Inputs`. |
| `unsafe-path` | A path is absolute, escapes its root, or uses backslashes. |
| `duplicate-path` | Two outputs write the same file. |
| `no-match` | A scan's directory pattern matched no directories, or a partial pattern matched no files. |
| `go-syntax` | A scanned Go file does not parse. |
| `template-syntax` | A template does not parse. |
| `template-exec` | A template failed while rendering, for example by referencing a missing field. |
| `invalid-output` | A template rendered Go that does not parse. |
| `invalid-directive` | An `//adder:` directive is malformed or repeated on one declaration. |
| `unknown-generator` | `adder new` was given a generator that `adder.yaml` does not declare. |
| `missing-input` | A required generator input was not given. |
| `invalid-input` | A generator input has the wrong type, fails a restriction, or is not declared. |
| `file-exists` | A generator would create a file that already exists. |

**Why:** error text is for people and changes freely, but codes and positions are for tools. Keeping both from the start means JSON output, editor squiggles, and documentation links need no rework later.

### 5. Paths in the configuration

Every path in `adder.yaml` is slash-separated, relative, and confined to its root. Scan directories and output paths are relative to the repository root; template paths are relative to the directory holding `adder.yaml`. Paths are cleaned on load, so `./drivers/*` and `drivers/*` are the same.

**Why:** the same configuration must behave the same on every operating system, and generated headers name the template, so absolute or machine-specific paths would make output differ between machines.

### 6. Item fields mean one thing

`.Package` is the package name from the file's `package` clause. `.Dir` is the directory. They usually match, but not always: directory `go-redis` may hold `package redis`.

**Why:** once templates depend on a field, its meaning cannot change. Getting it right before release avoids a confusing pair such as `.Package` and `.PackageName` forever.

### 7. Deterministic output

The same repository and configuration produce byte-identical output everywhere:

- Scans run in name order, and their items are sorted by name, file, and receiver.
- Template line endings are normalized to `\n`, so a Windows checkout with CRLF endings renders the same.
- Output never includes timestamps, user names, or absolute paths.

### 8. The generated-code header is frozen

Go outputs carry `// Code generated by adder from <template>. DO NOT EDIT.`, which satisfies [Go's convention](https://go.dev/s/generatedcode). `header.Format` writes it and `header.Parse` reads it, from the same constants.

**Why:** adder finds its own outputs by their header. Scans already use it to skip adder's outputs, so they never feed back into the next run, and future commands such as `adder explain` and stale-output cleanup will too. Changing the format would orphan every file generated before the change.

### 9. Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success. |
| 1 | adder ran and found a problem. |
| 2 | adder was invoked incorrectly, such as an unknown flag or command, or a missing or invalid generator input. |
| 3 | `adder check` found derived outputs that are out of date. |

Stale outputs get their own code so CI can tell "regenerate and commit" apart from "something is broken".

### 10. Everything is internal until the library API is designed

All packages live under `internal/`, so their APIs can change freely until v0.5, when the public library API is planned.

### 11. Go versions and dependencies

adder supports the two most recent Go releases. `go.mod` declares the older one in its `go` line, the minimum anyone importing adder needs, and the newest in its `toolchain` line for development. CI tests both.

Dependencies are kept to a minimum: cobra for the command line, `go.yaml.in/yaml/v3` for configuration, and `golang.org/x/mod` for reading `go.mod`. Anything that shapes generated output, such as template functions and case conversion, is implemented in adder itself, because a dependency upgrade must never change what people's templates produce.

### 12. One configuration file at the root

The configuration is a single, visible `adder.yaml` at the repository root. Only that name is accepted. adder finds it by walking up from the current directory, the way `go` finds `go.mod`, and its directory is the root. `--root` looks only in that directory, and `--config` names the file, making its directory the root unless `--root` is also given. Templates can live anywhere; their paths are relative to the configuration file.

**Why:** a root file is the norm for Go tools (`sqlc.yaml`, `buf.yaml`, `gqlgen.yml`, `.golangci.yml`), and a visible name signals that the file describes the repository's conventions rather than a tool's settings. Accepting other names later is additive; dropping one would break people.

### 13. Our own template functions, with Sprout's names

adder implements its template functions itself, using [Sprout](https://github.com/go-sprout/sprout)'s names wherever Sprout has an equivalent (`toSnakeCase`, `toCamelCase`, `toPascalCase`, `uniq`, `sortAlpha`), plus Go-specific case functions (`toGoPascalCase`, `toGoCamelCase`). Functions that are not deterministic, such as the current time, random values, or environment variables, are never included.

**Why:**

- Sprout's names are descriptive and correct. Sprig, its predecessor, names its PascalCase function `camelcase`, which matters in a Go generator where the first letter decides whether an identifier is exported.
- Depending on Sprig or Sprout would tie template behavior, and so people's output, to another project's release cycle. Sprout removed deprecated functions in a minor release.
- Neither library knows Go initialisms such as `ID` and `HTTP`.

Where adder needs a function neither library has, it picks a name that cannot be confused with theirs: `fieldValues` reads a field from a list of structs, unlike Sprig's map-based `pluck`.

### 14. Our own case conversion

Word splitting and case conversion live in `internal/render/textcase.go`:

- Words are separated by any rune that is not a letter or digit, at a lowercase-or-digit to uppercase change (`userID`), and before the last capital of an acronym followed by lowercase (`HTTPServer` is `HTTP` and `Server`).
- Digits stay with the word they follow (`HTTP2Server` is `HTTP2` and `Server`).
- The Go variants keep words that are already in capitals and capitalize golint's list of common initialisms.

**Why:** case conversion is part of the output, so it is covered by the compatibility promise either way. Owning roughly a hundred lines with thorough tests is cheaper than pinning and policing a dependency.

### 15. Stale outputs are found by header, within each output's own path (planned, v0.3)

When an output produces one file per item, adder removes files it previously generated that the current plan no longer produces. It looks only at files matching that output's path pattern, and only at files whose adder header names that output's template. Files without adder's header are never touched, and outputs removed from the configuration entirely are reported by `adder check` but not deleted.

| Considered | Why not |
| --- | --- |
| A manifest or lockfile | State in the repository that can drift, conflicts in busy repositories, and awkward when outputs are not committed. |
| Searching the whole repository for headers | Slow, and cannot tell which configuration owns a file in a monorepo. |
| An owned output directory, deleted before generating | Only works when generated files live apart from handwritten code. |

### 16. Comment directives use Go's directive syntax (planned, v0.2)

Directives look like `//adder:register name=s3 desc="Local disk"`: no space after `//`, a lowercase name, and `key=value` arguments with Go-style quoting when needed. Only directives in a declaration's doc comment count. A malformed directive, a directive repeated on one declaration, or an argument given twice is an `invalid-directive` problem rather than something adder guesses about. Bare words without `=` are rejected for now, so they stay available for a future meaning.

**Why:** Go's doc comment rules treat `//name:value` lines as directives, which godoc hides and gofmt keeps together, and the tools Go developers use most follow the same form: `//go:generate`, `//go:embed`, `//nolint:`, `//lint:ignore`. The `// +marker:` style from Kubernetes generators appears as text in documentation and is rarely used outside that ecosystem.

### 17. Type-aware scans are an explicit opt-in (planned, v0.4)

A scan uses type information only when it sets `typeCheck: true`. Filters that need it, such as "implements this interface", fail validation on a scan without it, with a hint to enable it. Type-aware scans run the Go toolchain offline (`GOPROXY=off`, `-mod=readonly`) and report missing dependencies instead of downloading them.

**Why:** type checking is much slower than parsing, needs code that compiles and dependencies in the module cache, and runs the toolchain. Enabling it automatically based on which filters are used would hide a performance and reliability cliff behind an innocent-looking change. Parsing stays the fast default.

### 18. The library API arrives when it has a real consumer (planned)

The module's root package, `github.com/TyphonHill/adder`, is reserved for a small public facade over the internal packages, such as `adder.Load`, `adder.Derive`, and a way to register custom functions and scanners. It ships as an experimental v0.x API once a real consumer needs it, such as a repository that wants a custom scanner or an editor integration. The internal packages stay internal.

**Why:** moving code is easy; committing to an API is not. After v1.0 a breaking change needs a new `/v2` import path, and today's internal APIs are shaped by a single consumer, the command line. They changed completely in one refactoring, which would have broken every user had they been public. `cmd/adder` is already public, because `go get -tool` depends on its path.

### 19. Scaffolding with `adder new` (planned, v0.2)

Generators create new files that people then own. They live next to `derive` in `adder.yaml`:

```yaml
version: 1

vars:
  License: Apache-2.0

derive:
  scans:
    Drivers:
      dirs: ["drivers/*"]
      funcs: {exported: true, name: ^New$}
  outputs:
    - template: templates/drivers.go.tmpl
      path: drivers/drivers.gen.go

generators:
  driver:
    description: Add a storage driver
    inputs:
      name:
        help: Driver name, used as the package name
        pattern: ^[a-z][a-z0-9]*$
        unique: Drivers.Package
      summary:
        help: One-line description for the package documentation
        default: A storage driver.
      cache:
        type: bool
        help: Add an in-memory cache in front of the driver
    files:
      - template: templates/new/driver.go.tmpl
        path: drivers/{{ .Inputs.name }}/{{ .Inputs.name }}.go
      - template: templates/new/driver_test.go.tmpl
        path: drivers/{{ .Inputs.name }}/{{ .Inputs.name }}_test.go
      - template: templates/new/cache.go.tmpl
        path: drivers/{{ .Inputs.name }}/cache.go
        when: cache
    message: |
      Next, implement New in drivers/{{ .Inputs.name }}/{{ .Inputs.name }}.go.
```

```console
$ adder new driver --name gcs --cache
created    drivers/gcs/gcs.go
created    drivers/gcs/gcs_test.go
created    drivers/gcs/cache.go
updated    drivers/drivers.gen.go

Next, implement New in drivers/gcs/gcs.go.
```

**Generators.** Each entry under `generators` is a command: `adder new <name>`. Names are lowercase words joined by hyphens, like command names.

**Inputs.**

- Input names are Go identifiers in lowerCamelCase. Each one becomes a flag in kebab-case (`packageName` becomes `--package-name`) and the template field `.Inputs.packageName`.
- `type` is `string`, the default, or `bool`. Numbers and lists are left out until a real generator needs them; a string with a `pattern` covers most cases.
- A string input without a `default` is required. Bool inputs default to `false` and are never required, so there is no `required` setting.
- `pattern` must match the whole value. `values` lists the allowed values, and `valuesFrom: Drivers.Package` takes them from a scan. `unique: Drivers.Package` rejects values a scan already has, so two drivers cannot share a name.
- Values are validated, never transformed. A `transform` setting would hide what was typed; templates have case functions for that.
- Values come from flags, from a JSON object with `--inputs file.json`, or from standard input with `--inputs -`. Flags override the file. Unknown keys are errors.

**Files.**

- `template` is relative to the configuration file's directory. `path` is a template rendered with the same data and must stay inside the root after rendering.
- `when: cache` and `unless: cache` include a file depending on a bool input. They take an input name, not an expression, so the configuration never becomes a programming language.
- `ifExists` is `fail`, the default, or `skip`. There is no `overwrite`: once a file exists, people own it ([principle 4](../ROADMAP.md#principles)).
- Go files are gofmt-formatted but do not get a generated-code header, because they are meant to be edited.

**Template data.** Generator templates see `.Inputs`, `.Vars`, `.Module`, and every scan by name. Derive templates see `.Vars`, `.Module`, and the scans. `Inputs` and `Vars` join `Module` as reserved scan names.

**One plan for everything.** `adder new` builds a single plan containing the new files and every derived output that changes because of them. Derived outputs are recomputed against the repository with the new files overlaid in memory, so registries pick up the new code in the same run. Nothing is written unless the whole plan succeeds. `derive: false` on a generator opts out of the derive step.

**Previewing.** `--dry-run` prints the plan without writing. `--diff` also prints a unified diff of every change, with new files shown as additions, and implies `--dry-run`, like `gofmt -d`. Paths use `a/` and `b/` prefixes and new files come from `/dev/null`, so the output is a patch that `git apply` and `patch -p1` accept. The diff code is copied from the Go toolchain's `internal/diff` (BSD-3-Clause, about 250 lines) into `internal/diff`, with its license header, rather than added as a dependency.

**Command line.** Generators define their own flags, so `adder new` loads `adder.yaml` before parsing the rest of the command line, and builds a subcommand for each generator. Each one gets its own `--help`, flag parsing, and suggestions for mistyped names from the same library as every other command. Inputs cannot use a flag `adder new` defines itself (`--config`, `--diff`, `--dry-run`, `--help`, `--inputs`, `--root`), and two inputs cannot map to the same flag. In a JSON inputs file, keys are input names, not flags, so `{"packageName": "x"}`.

**Writing.** New files are created exclusively: a file that appears between planning and writing makes the run fail rather than be overwritten. `valuesFrom` and `unique` refer to a string field of scan items, checked when the generator runs.

**Problems.** New diagnostic codes are `unknown-generator`, `missing-input`, `invalid-input`, and `file-exists`. Mistakes in how a generator was invoked, such as a missing or invalid input, exit with status 2. A file that already exists exits with status 1. The plan gains a `skipped` status for files left alone by `ifExists: skip`.

**Shared configuration.** Two smaller additions serve generators and derived outputs alike:

- `vars` at the top level holds string values available to every template as `.Vars`, such as a license name. Names are Go identifiers. `.Vars` is present even when no vars are set, so templates can always use it.
- `partials` lists glob patterns of templates loaded into every template, so `{{ template "license" . }}` works anywhere. Partials are defined with `{{ define }}` and never rendered on their own.
  - Patterns use `path.Match` syntax without `**`, are relative to the configuration file's directory, and must each match a file. Directories they match are ignored.
  - Two partials defining the same name is an error, since which one wins would depend on file order. A template may redefine a partial's block for itself; each template renders from its own copy of the partials.
  - Problems inside a partial are reported at the partial's own file and line.

| Considered | Why not |
| --- | --- |
| Inputs at the top level of template data (`.name`) | Collides with scan names and leaves no room for new data; `.Inputs.name` is explicit. |
| A `required` setting | Redundant: a string without a default is required. |
| Conditions written as template expressions | Turns `adder.yaml` into a programming language. |
| An `overwrite` file policy | Breaks the promise that files people own are never rewritten. |
| A diff library | A small, stable algorithm is better copied than depended on. |

### 20. Richer scans without type checking (planned, v0.2)

Go source alone says far more than names, so these additions parse files rather than type-check them.

**Directories.**

- `**` matches any number of directories: `internal/**/handlers`. Like `go`'s `./...`, it skips `testdata`, `vendor`, and directories whose names start with `.` or `_`.
- `exclude` lists patterns of directories to leave out, in the same syntax as `dirs`.

**Function filters.** These join `exported`, `firstParam`, and `name`:

- `kind: func` or `kind: method` limits a scan to plain functions or to methods.
- `receiver: Worker` matches methods on `Worker` or `*Worker`.
- `firstParamType: context.Context` matches the first parameter's type as written, with its package resolved through the file's imports, so `ctx context.Context` and `c context.Context` both match. Types are written as Go writes them, with each package replaced by its full import path: `go.temporal.io/sdk/workflow.Context`, `*example.com/m/config.Config`, `[]string`. An import without an alias is assumed to use the package name goimports assumes: the last element of its path, without a major version suffix such as `/v2` or `.v3` and without a `go-` prefix. Packages that break that convention need type checking.
- `directive: register` matches declarations whose doc comment has `//adder:register`.

**Type declarations.** A scan can select types instead of functions, with a `types` block in place of `funcs`. It supports `exported`, `name`, `directive`, and `kind` (`struct`, `interface`, or any other type). Each scan selects exactly one kind of declaration.

**Item fields.** Items gain:

| Field | Description |
| --- | --- |
| `.ImportPath` | The package's import path: the module path joined with `.Dir`. Empty when the repository has no `go.mod`. |
| `.Kind` | `func`, `method`, `struct`, `interface`, or `type`. |
| `.Doc` | The doc comment, without directives. |
| `.Line` | The line of the declaration. |
| `.Directives` | Arguments of each `//adder:` directive, by name: `{{ index .Directives.register "name" }}`. A directive repeated on one declaration is an error. |
| `.Signature` | Parameters and results as written, such as `(ctx context.Context, name string) error`. |
| `.Params`, `.Results` | Each parameter and result, with `.Name` and `.Type` as written. |
| `.Fields` | For structs, each field with `.Name`, `.Type`, and `.Tag`. |

Items are a single type whose fields are empty when they do not apply, so templates and `fieldValues` work the same for every kind of declaration.

## Compatibility

From v1.0, semantic versioning covers:

- The `adder.yaml` schema for a given `version`.
- Template data: scan item fields, `.Module`, and template function names and behavior.
- The generated-code header format.
- Diagnostic codes, exit codes, and JSON output.

It does not cover the wording of messages or the human-readable output of commands, which may improve in any release.

Before v1.0, any of these may change, and the changelog will say so.
