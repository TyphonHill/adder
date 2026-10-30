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
| `internal/imports` | Package names for import paths, and removing unused imports from rendered Go. |
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
| `reserved-name` | A scan name collides with data adder provides: `Module`, `Vars`, `Inputs`, or `Item`. |
| `unsafe-path` | A path is absolute, escapes its root, uses backslashes, or goes through a symbolic link. |
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
| `usage` | With `--json`, adder was invoked incorrectly, such as with an unknown flag. |

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

**Ownership:** `derive` never replaces a file that lacks adder's header with an output that has one. It reports `file-exists` instead, so a handwritten file at an output's path, or one a new scan item would land on, is never lost. Any template may be named, so renaming a template keeps working. When the output has no adder header, because its file type has no comments or its template writes its own generated-code comment, adder cannot tell its files from others and replaces them as before.

Outside Go, adder's header counts only on the first line after any preamble, where adder writes it. That decides which files stale cleanup removes, so a header quoted further down, as in documentation, never makes a file look generated.

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

Dependencies are kept to a minimum: cobra for the command line, `go.yaml.in/yaml/v3` for configuration, `golang.org/x/mod` for reading `go.mod`, and `github.com/rogpeppe/go-internal/diff` for `--diff` previews. Anything that shapes generated output, such as template functions and case conversion, is implemented in adder itself, because a dependency upgrade must never change what people's templates produce.

Planned for v0.3: `golang.org/x/sys`, only to turn on color in Windows consoles ([decision 32](#32-terminal-output-planned-v03)). Tests also use `github.com/rogpeppe/go-internal/testscript`, from the same module as the diff code, which is never built into the binary.

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

Word splitting and case conversion live in `internal/textcase`:

- Words are separated by any rune that is not a letter or digit, at a lowercase-or-digit to uppercase change (`userID`), and before the last capital of an acronym followed by lowercase (`HTTPServer` is `HTTP` and `Server`).
- Digits stay with the word they follow (`HTTP2Server` is `HTTP2` and `Server`).
- The Go variants keep words that are already in capitals and capitalize golint's list of common initialisms.

**Why:** case conversion is part of the output, so it is covered by the compatibility promise either way. Owning roughly a hundred lines with thorough tests is cheaper than pinning and policing a dependency.

### 15. Stale outputs are found by header, within each output's own path (planned, v0.3)

When an output produces one file per item ([decision 24](#24-one-output-per-item-planned-v03)), adder removes files it previously generated that the current plan no longer produces. It looks only at files matching that output's path pattern, and only at files whose adder header names that output's template. Files without adder's header are never touched.

Files left behind by an output that was removed from the configuration are not found, because finding them would mean reading every file in the repository. `adder explain` reports such a file as belonging to no output, so it can be deleted by hand.

| Considered | Why not |
| --- | --- |
| A manifest or lockfile | State in the repository that can drift, conflicts in busy repositories, and awkward when outputs are not committed. |
| Searching the whole repository for headers | Slow, and cannot tell which configuration owns a file in a monorepo. |
| An owned output directory, deleted before generating | Only works when generated files live apart from handwritten code. |

### 16. Comment directives use Go's directive syntax

Directives look like `//adder:register name=s3 desc="Local disk"`: no space after `//`, a lowercase name, and `key=value` arguments with Go-style quoting when needed. Only directives in a declaration's doc comment count. A malformed directive, a directive repeated on one declaration, or an argument given twice is an `invalid-directive` problem rather than something adder guesses about. Bare words without `=` are rejected for now, so they stay available for a future meaning.

**Why:** Go's doc comment rules treat `//name:value` lines as directives, which godoc hides and gofmt keeps together, and the tools Go developers use most follow the same form: `//go:generate`, `//go:embed`, `//nolint:`, `//lint:ignore`. The `// +marker:` style from Kubernetes generators appears as text in documentation and is rarely used outside that ecosystem.

### 17. Type-aware scans are an explicit opt-in (planned, v0.4)

A scan uses type information only when it sets `typeCheck: true`. Filters that need it, such as "implements this interface", fail validation on a scan without it, with a hint to enable it. Type-aware scans run the Go toolchain offline (`GOPROXY=off`, `-mod=readonly`) and report missing dependencies instead of downloading them.

**Why:** type checking is much slower than parsing, needs code that compiles and dependencies in the module cache, and runs the toolchain. Enabling it automatically based on which filters are used would hide a performance and reliability cliff behind an innocent-looking change. Parsing stays the fast default.

### 18. The library API arrives when it has a real consumer (planned)

The module's root package, `github.com/TyphonHill/adder`, is reserved for a small public facade over the internal packages, such as `adder.Load`, `adder.Derive`, and a way to register custom functions and scanners. It ships as an experimental v0.x API once a real consumer needs it, such as a repository that wants a custom scanner or an editor integration. The internal packages stay internal.

**Why:** moving code is easy; committing to an API is not. After v1.0 a breaking change needs a new `/v2` import path, and today's internal APIs are shaped by a single consumer, the command line. They changed completely in one refactoring, which would have broken every user had they been public. `cmd/adder` is already public, because `go get -tool` depends on its path.

### 19. Scaffolding with `adder new`

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

**Previewing.** `--dry-run` prints the plan without writing. `--diff` also prints a unified diff of every change, with new files shown as additions, and implies `--dry-run`, like `gofmt -d`. Paths use `a/` and `b/` prefixes and new files come from `/dev/null`, so the output is a patch that `git apply` and `patch -p1` accept. Diffs come from `github.com/rogpeppe/go-internal/diff`, which exports the Go toolchain's own `internal/diff`, so previews match `gofmt -d`. It is an anchored diff: it matches regions around lines that appear once in both texts, so it never pairs unrelated blank lines or closing braces, and it always runs in O(n log n) time.

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
| Copying Go's `internal/diff` into adder | go-internal already exports the same code and keeps it in step with Go, so a copy only adds maintenance. adder copied it in v0.2, before depending on go-internal. |
| Other diff libraries | Myers diffs that differ from `gofmt -d`, Python difflib semantics, or character-level diffs that `git apply` cannot read. |

### 20. Richer scans without type checking

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

### 21. Unused imports are removed, missing ones are not added

Rendered Go files lose the imports they never refer to, after `gofmt`. Templates can list every import a file might need instead of wrapping each in `{{ if }}`. A package is referred to by its explicit name, or by the name goimports assumes from its path: the last element, skipping a major version such as `v2` and a `go-` prefix. Blank, dot, and cgo imports are always kept, and a name that might be a local variable keeps its import.

Missing imports are not added. That needs to know which package a name like `rand` or `template` means, and goimports answers it by searching the module cache, so the output would depend on the machine. A built-in table of the standard library would make it deterministic, at the cost of output that can change between adder releases.

| Considered | Why not |
| --- | --- |
| `golang.org/x/tools/imports` | Output would depend on the machine's module cache and the dependency's version ([decision 11](#11-go-versions-and-dependencies)). |
| Pruning by type checking | Needs the packages to build, which generated code often cannot do before it exists. |

**Why:** an unused import is a compile error, so removing one only changes output that was already broken, which makes it safe as a default.

### 22. JSON output (planned, v0.3)

Every command accepts `--json`. It writes exactly one JSON object to standard output and nothing to standard error, whether the command succeeds or not, and exit codes stay as they are. Problems go in the object instead of being printed:

```json
{
  "changes": [
    {"path": "drivers/drivers.gen.go", "status": "updated"},
    {"path": "drivers/registry.gen.go", "status": "unchanged"}
  ],
  "problems": [
    {
      "code": "go-syntax",
      "message": "expected 'IDENT', found '{'",
      "pos": {"file": "drivers/s3/s3.go", "line": 3, "column": 6}
    }
  ]
}
```

- `problems` is always present, empty on success. Problems are the same `diag.Diagnostic` values the text output prints, with positions written the same way ([decision 4](#4-structured-diagnostics)).
- `changes` lists every file in the plan, unchanged ones included, so a tool sees the whole picture without a second command. Change paths are slash-separated and relative to the root. With `--diff`, each change that is not unchanged also has a `diff` field holding its unified diff.
- `adder derive` and `adder new` add `dryRun`, since `--diff` implies it, and `adder new` adds `generator` and `message`. The README lists every command's fields.
- Mistakes in how adder was invoked, such as an unknown flag, become a problem with the new code `usage`, so a tool never has to parse help text. `--help` still prints text.
- Field names are lowerCamelCase. From v1.0, fields are only ever added.

**Why:** agents and scripts drive adder through this output ([principle 8](../ROADMAP.md#principles)). One object per run is simpler to consume than a stream, and adder plans everything before writing, so there is nothing to stream. `--json` matches `go list -json`, `go vet -json`, and `gh --json`.

| Considered | Why not |
| --- | --- |
| `--format json` or `-o json` | Only one machine format is planned. A `--format` flag can be added later for others, such as GitHub annotations. |
| Problems on standard error as JSON lines | Two streams to read and interleave, for no gain when the run produces one result. |
| A version field in the object | The output follows adder's semantic versioning like everything else in the compatibility promise. |

### 23. Discovering generators and outputs (planned, v0.3)

Three read-only commands let people and agents learn what a repository offers without reading `adder.yaml`.

**`adder list`** prints every generator with its description, and every derived output with its template and path:

```console
$ adder list
generators
  driver    Add a storage driver
outputs
  drivers/drivers.gen.go    from templates/drivers.go.tmpl
```

**`adder describe <generator>`** prints a generator's inputs, its files, and its message. Allowed values from `valuesFrom` and existing values for `unique` are computed from the scans, so they are current. With `--json`, it includes `inputSchema`, a JSON Schema (draft 2020-12) for the object `--inputs` accepts. That is the shape MCP uses for tool inputs, so an agent can fill it in directly:

| Input setting | JSON Schema |
| --- | --- |
| `type: string` / `type: bool` | `"type": "string"` / `"type": "boolean"` |
| `help` | `description` |
| `default` | `default` |
| no `default` on a string | listed in `required` |
| `pattern` | `pattern`, anchored as adder anchors it. adder validates with Go's regular expressions, which differ from JSON Schema's ECMAScript dialect only in rarely used syntax. |
| `values`, `valuesFrom` | `enum` |
| `unique` | `"not": {"enum": [...]}` with the values that already exist |

The schema sets `additionalProperties: false`, as `--inputs` rejects unknown keys.

**`adder explain <file>`** says where a file comes from: the output in `adder.yaml` and its position, the template, the scans the template refers to by name, and whether the file is up to date. A file without adder's header is reported as not generated by adder. One whose header names a template no output uses is reported as belonging to no output ([decision 15](#15-stale-outputs-are-found-by-header-within-each-outputs-own-path-planned-v03)). Scans are found by reading the template's parse tree, including its partials, for fields named after a scan. A template that reaches a scan indirectly, such as through `index`, is not attributed to it.

**Why:** an agent that can list, describe, preview with `--dry-run --json`, and run a generator never has to guess a repository's conventions. `explain` answers the first question anyone asks about a generated file: what do I edit instead?

### 24. One output per item (planned, v0.3)

An output with `each` renders its template once for every item of a scan, with the item available as `.Item`. Its `path` is then a template rendered for each item:

```yaml
derive:
  outputs:
    - template: templates/mock.go.tmpl
      each: Stores
      path: "{{ .Item.Dir }}/{{ .Item.Name | toSnakeCase }}_mock.gen.go"
```

- `each` names a scan. Templates still see every scan, `.Vars`, and `.Module`. `Item` joins the reserved scan names.
- Two items rendering to the same path is a `duplicate-path` problem, so a path that does not use `.Item` fails as soon as a scan has two items.
- `path` is only a template on outputs with `each`. A single output's path is written literally, so it can never move without the configuration changing.
- Each output with `each` must have a template no other output uses, and a path whose file type has a generated-file header ([decision 25](#25-generated-file-headers-for-other-languages-planned-v03)), because stale files are found by that header.

**Stale files.** Files the output no longer produces are removed ([decision 15](#15-stale-outputs-are-found-by-header-within-each-outputs-own-path-planned-v03)). The candidates are files whose path matches the output's `path` with each template action replaced by a wildcard that may span directories, and whose header names the output's template. adder searches from the longest directory in `path` before the first action, and skips `testdata`, `vendor`, and hidden directories as `**` does.

The plan gains a `removed` status. `adder derive` deletes removed files, `adder check` treats them as stale, and `--diff` shows them as deletions to `/dev/null`.

| Considered | Why not |
| --- | --- |
| A templated path on every output | A single output whose path depends on data could move and leave its old file behind. |
| Asking for a separate glob of files the output owns | Repeats the path in a second form that can drift from it. |

### 25. Generated-file headers for other languages (planned, v0.3)

Outputs in languages other than Go get adder's header in their own comment syntax, chosen by file extension from a built-in table. The text is the same as the Go header, so `Code generated by adder from <template>. DO NOT EDIT.` marks every generated file:

| Comment | Files |
| --- | --- |
| `// ...` | Go (unchanged), C, C++, C#, Dart, Java, JavaScript, Kotlin, Protocol Buffers, Rust, Scala, Swift, TypeScript |
| `# ...` | Dockerfile, HCL and Terraform, Makefile, Nix, Perl, PowerShell, Python, R, Ruby, shell scripts, TOML, YAML |
| `-- ...` | Haskell, Lua, SQL |
| `/* ... */` | CSS, Sass |
| `<!-- ... -->` | HTML, Markdown, SVG, Vue, XML |

- The header goes on the first line, or after a leading `#!` line, an XML declaration, or Markdown front matter, which must stay first.
- A file whose leading comments, before its first line of content, include a `Code generated ... DO NOT EDIT.` line keeps it and gets no second header. This mirrors Go's rule, which only counts that comment before the package clause.
- File types without comments, such as JSON, and extensions not in the table get no header. They cannot use `each`, and `adder explain` cannot identify them.
- The table is part of the compatibility promise. Adding a file type changes output, so it is listed under **Changed** in the changelog.

**Why:** tools from linguist to code review treat files with this marker as generated, and adder needs the header to find its own files. A built-in table keeps the configuration free of comment syntax.

### 26. Symlinks (planned, v0.3)

- Scans skip symbolic links to files and directories alike, as `go` skips linked directories in `./...`. This also rules out loops.
- adder never writes, creates, or removes a file through a symbolic link. An output or new file whose path, or any directory on it, is a link inside the root is an `unsafe-path` problem. `os.Root` already rejects links that lead outside the root.
- Templates, partials, and `adder.yaml` may be links, since reading them cannot change anything.

**Why:** writing through a link changes a file somewhere else, which may be one that people own ([principle 4](../ROADMAP.md#principles)).

### 27. Build constraints in scans (planned, v0.3)

Scans skip files whose build constraints can never be satisfied, such as `//go:build ignore`. Constraints are evaluated the same way on every machine: custom tags are false, and `GOOS`, `GOARCH`, `unix`, `cgo`, and Go version tags may take any value. A file is skipped only when no combination of those satisfies it. `buildTags` on a scan lists custom tags to treat as true, such as `integration`. File name suffixes such as `_linux.go` are platform constraints, so they never exclude a file.

**Why:** evaluating constraints for the machine adder runs on would make output differ between a Mac and a Linux CI runner ([decision 7](#7-deterministic-output)). Files for one platform belong in the scan, and files marked `ignore` or for a custom tag usually do not.

### 28. More template functions (planned, v0.3)

Functions Sprout also has keep its names and argument order, with the value last, so they work at the end of a pipeline:

| Function | Example | Result |
| --- | --- | --- |
| `join` | `join ", " .Names` | `a, b` |
| `trimPrefix` | `trimPrefix "Handle" .Name` | `Users` for `HandleUsers` |
| `trimSuffix` | `trimSuffix "Handler" .Name` | `Users` for `UsersHandler` |
| `replace` | `replace "-" "_" .Name` | `go_redis` for `go-redis` |
| `quote` | `quote .Name` | `"s3"`, quoted as Go writes strings |
| `indent`, `nindent` | `nindent 4 .Doc` | each line indented by four spaces, after a newline with `nindent` |

`quote` takes exactly one value, where Sprout's joins several, so a template written for Sprout fails rather than renders differently.

Functions Sprout lacks get names that do not collide with it:

| Function | Example | Result |
| --- | --- | --- |
| `pluralize`, `singularize` | `pluralize .Name` | `Drivers` for `Driver`, `IDs` for `ID` |
| `groupBy` | `groupBy "Dir" .Handlers` | a list of `.Key` and `.Items`, sorted by key |

- `pluralize` and `singularize` use English rules with a built-in list of irregular words, and keep the case of the word: an all-capitals word gets a lowercase suffix, as Go writes `IDs`. Sprout's `plural` instead picks one of two given words by a count, and is not included.
- `groupBy` takes a string field, like `fieldValues`, and returns a list rather than a map, so groups keep a stable order and work with `range` and `fieldValues`.

### 29. Prompts for missing inputs (planned, v0.3)

When `adder new` is missing a required input, standard input and standard error are both terminals, and neither `--json` nor `--inputs -` is given, adder asks for the value instead of failing:

```console
$ adder new driver
name: Driver name, used as the package name
  must match ^[a-z][a-z0-9]*$, and not be one of: disk, s3
> gcs
```

- Only required inputs are asked for. Optional inputs and bools keep their defaults, so the result is the same as running with flags.
- Inputs with `values` or `valuesFrom` list the choices, numbered, and accept a number or a value.
- A value that fails validation is explained and asked for again. Nothing is written until every input is valid, and Ctrl-C cancels.
- Prompts are plain lines, with no arrow-key menus.

**Why:** a person running a generator by hand should not have to look up its flags first, while scripts, CI, and agents, which have no terminal, keep getting a `missing-input` problem.

| Considered | Why not |
| --- | --- |
| A prompt library such as huh or survey | Several dependencies, raw terminal handling, and Windows differences, for menus that line prompts cover. |
| Asking for every input | Tedious for generators with many optional inputs, and gives a different result than the same command with flags. |

### 30. Generator tests (planned, v0.3)

A generator can declare tests: sample inputs and the files it should create.

```yaml
generators:
  driver:
    # inputs, files, and message as before
    tests:
      - name: with-cache
        inputs: {name: example, cache: true}
        want: testdata/generators/driver/with-cache
```

`adder test` runs every test, or only those of the generators it is given. Each test plans its generator against the repository like `--dry-run` and compares the files the generator creates with the files under `want`, which mirror their paths in the repository. A missing, extra, or different file fails the test with a diff and the new code `golden-mismatch`, and `adder test` exits with status 1. `adder test --update` rewrites the `want` directories instead.

- `want` is relative to the configuration file's directory. Under a `testdata` directory, `**` scans and Go tooling ignore it.
- Inputs are validated like an `--inputs` file, against the generator's inputs.
- Derived outputs are not compared, because they depend on everything in the repository and would fail whenever unrelated code changed.
- Tests run against the real repository, so `unique` and existing files still apply. Test inputs should use names that will not exist, such as `example`.

**Why:** a generator is code that a team relies on, and templates break silently when a scan field or function changes. Golden files make a generator's output reviewable in pull requests.

### 31. Checking the configuration (planned, after v1.0)

**A JSON Schema for `adder.yaml`.** The schema lives at `schema/v1.json` for `version: 1`, written in draft-07, which editors support best. `adder init` writes a `# yaml-language-server: $schema=` line pointing at the file on `main`, `https://raw.githubusercontent.com/TyphonHill/adder/main/schema/v1.json`, so the path must never move. A SchemaStore listing for files named `adder.yaml` waits until after v1.0, when the schema is stable. The schema is written by hand, so it can carry descriptions and examples, and a test checks that its properties match the configuration types exactly. It is a convenience for editors: adder's own validation stays the authority, since it reports positions and checks references between settings.

**`adder validate`** checks everything except whether outputs are up to date: the configuration, every template and partial, generator paths and tests, the scans, and rendering every derived output. It writes nothing and exits with status 0 or 1.

**Why:** `adder check` needs outputs to be committed to say anything. Repositories that generate outputs at build time still want CI to catch a broken template, and people editing `adder.yaml` want a quick answer.

### 32. Terminal output (planned, v0.3)

- `--quiet` prints only problems. `--verbose` also lists unchanged files and how many items each scan found. They cannot be combined.
- Status words and problem codes are colored when the stream is a terminal, `NO_COLOR` is unset or empty, `TERM` is not `dumb`, and `--json` is not given. On Windows, adder enables the console's color support through `golang.org/x/sys`, and prints without color if that fails.
- `adder completion bash|zsh|fish|powershell` prints a completion script. It completes generator names with their descriptions, the flags of each generator, input values from `values` and `valuesFrom`, and generated files for `adder explain`. Completion reads `adder.yaml` and runs scans, but never writes. It completes nothing when the configuration does not load, rather than printing errors into the shell.

**Why:** these follow what people expect from a modern command line, and none of them changes what adder writes.

### 33. Releases (planned, after v1.0)

Pushing a `v*` tag runs GoReleaser in GitHub Actions:

- **Binaries:** builds for Linux, macOS, and Windows on amd64 and arm64, with `-trimpath`, `CGO_ENABLED=0`, and file times taken from the commit, so rebuilding a tag produces identical archives. The version comes from the build information Go embeds from the tag, as `adder --version` already reads it, so no linker flags are needed.
- **Integrity:** `checksums.txt` is signed with keyless cosign, as a Sigstore bundle. Each archive has an SPDX SBOM and a GitHub artifact attestation with SLSA build provenance, which `gh attestation verify` checks.
- **Homebrew:** a cask in `TyphonHill/homebrew-tap`, pushed with a fine-grained token that can write only to that repository.
- **Release notes:** the changelog's section for the tag. The release fails when that section is missing, so no release ships without its changelog.
- **Permissions:** the release job alone gets `contents: write`, `id-token: write`, and `attestations: write`.

`go install` and `go get -tool` keep working from source as before.

| Considered | Why not |
| --- | --- |
| The SLSA GitHub generator | GitHub's artifact attestations give the same provenance natively, with less workflow. |
| A Homebrew formula | GoReleaser now publishes prebuilt binaries to Homebrew as casks and has deprecated formulas for them. |
| Version set with `-ldflags -X` | Duplicates what Go's build information already records. |

### 34. Integrations and coding agents (planned, after v1.0)

- **`go generate`:** one `//go:generate go tool adder derive` directive anywhere in the module is enough, since adder finds `adder.yaml` from any directory ([decision 12](#12-one-configuration-file-at-the-root)) and `derive` regenerates every output.
- **pre-commit:** `.pre-commit-hooks.yaml` defines `adder-check` and `adder-derive`, built from source with `language: golang`. They run on every commit and ignore the changed file names, because a change anywhere can affect a scan.
- **GitHub Action:** `action.yml` at the repository root runs `adder check` by default, or another command. It uses `go tool adder` when `go.mod` declares the tool, and otherwise runs the action's own version with `go run`. Downloading a signed binary instead waits for published releases ([decision 33](#33-releases-planned-after-v10)). Problems become annotations on the pull request, read from `--json`.
- **GitLab CI:** a documented job that runs `adder check` and turns `--json` problems into a Code Quality report.
- **Coding agents:** a guide in `docs/agents.md`, and an `AGENTS.md` section to copy into a repository. It tells agents to discover generators with `adder list --json` and `adder describe`, preview with `--dry-run --json`, run `adder derive` after adding code, never edit files marked `DO NOT EDIT`, and use `adder explain` to find the template to change instead.

### 35. Documentation and examples (planned, after v1.0)

- **Site:** built with Hugo and the Hextra theme from `site/` and deployed to GitHub Pages from `main`. Hugo is a single Go binary and Hextra installs as a Hugo module, so the documentation needs no Node toolchain. The site holds guides, references, recipes, a comparison with other tools, and a guide to migrating from a hand-rolled generator.
- **References are generated:** the configuration reference comes from the JSON Schema and the function reference from the function table, by a program in `internal/tools/docgen`. Function examples run as tests, and a test fails when the generated pages are out of date, the same way `adder check` does.
- **Examples:** `examples/` holds complete repositories, each its own module with `adder.yaml`, templates, Go code, and committed outputs: a driver registry, HTTP routes, CLI commands, and workflow engine wiring. CI runs `adder check` and `go build ./...` in each with adder built from the same commit. Recipes on the site walk through them.
- **Comparisons** state the version of each tool compared and the date, and link to its documentation.
- **Issues and pull requests** use templates: bug reports ask for `adder --version`, the relevant `adder.yaml`, the command, and its output.

Before then, the README and `docs/` remain the documentation.

**Why:** each example is its own module, so `.Module` and `.ImportPath` work as they do in a real repository, and adder's own module stays free of example code.

### 36. Testing adder itself (planned, v0.3)

- **End to end:** `testscript` scripts in `cmd/adder/testdata/script` run the real command against small repositories written inline, covering each command's text and JSON output and exit codes. Today's command tests stay as unit tests for details that scripts express poorly.
- **Fuzzing:** fuzz tests for configuration loading, header parsing, directive parsing, and import pruning check that no input panics, that problems point inside the input, and that pruning keeps gofmt-formatted Go parsing. Seed corpora run with every test run, and a weekly workflow fuzzes each target for five minutes.
- **Performance budget:** a benchmark builds a repository of 1,000 packages and 10,000 functions and runs `derive` over it. The budget is 500 ms on GitHub's standard runners. CI fails above four times the budget, which catches a slowdown by an order of magnitude without failing on noisy timing. Parallel parsing is only added if the budget is missed.

## Compatibility

From v1.0, semantic versioning covers:

- The `adder.yaml` schema for a given `version`.
- Template data: scan item fields, `.Module`, and template function names and behavior.
- The generated-code header format.
- Diagnostic codes, exit codes, and JSON output.

It does not cover the wording of messages or the human-readable output of commands, which may improve in any release.

Before v1.0, any of these may change, and the changelog will say so.
