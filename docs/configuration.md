# Configuration

`adder.yaml` sits at the root of a repository, or of each service in a [monorepo](commands.md#monorepos). Its directory is the root that scans and outputs are relative to. All paths use forward slashes and must stay inside the root.

```yaml
version: 1
vars: {}        # strings every template can read
partials: []    # templates whose {{ define }} blocks every template can use
derive:
  scans: {}     # what to read from the code
  outputs: []   # files rendered from the scans
generators: {}  # adder new <name>
packs: {}       # uses of packs, see extending.md
```

Unknown keys are errors, reported at their line, so a typo never passes silently.

## Scans

Each entry under `derive.scans` selects items from the code. Its name is the template field holding them, so it must be a Go identifier other than `Module`, `Vars`, `Inputs`, `Options`, or `Item`. A scan has one of `funcs` (the default), `types`, `values`, `files`, or [`scanner`](extending.md#scanners):

| Field | Description |
| --- | --- |
| `dirs` | Directories to scan. Globs such as `drivers/*` are allowed, and `**` matches any number of directories. Like `go`'s `./...`, wildcards skip `testdata`, `vendor`, and directories starting with `.` or `_`. |
| `exclude` | Directories to leave out, in the same syntax. `internal/legacy/**` excludes a directory and everything below it. |
| `includeTests` | Also scan `_test.go` files. |
| `buildTags` | Custom build tags to treat as set, such as `[integration]`. |
| `funcs` | Functions and methods: `exported`, `name` (a regular expression), `kind` (`func` or `method`), `receiver`, `firstParamName`, `firstParamType`, `directive`. |
| `types` | Type declarations: `exported`, `name`, `kind` (`struct`, `interface`, or `other`), `directive`, and with `typeCheck`, `implements`. |
| `values` | Constants and variables: `kind` (`const` or `var`), `exported`, `name`, `type`, `directive`. |
| `files` | Files of any kind, by `name`, a regular expression the file name must match, such as `\.sql$`. Contents are not read. |
| `typeCheck` | Load type information, for filters that compare types rather than spelling. See [type-aware scans](#type-aware-scans). |
| `platform` | With `typeCheck`, the `GOOS/GOARCH` to check. Defaults to `linux/amd64`. |

Unset filters match everything. `firstParamType` is written with full import paths, such as `context.Context` or `go.temporal.io/sdk/workflow.Context`. In a `const` group, a name without its own type repeats the one before it, as Go does, so every member of an `iota` enum matches `values.type`.

Scans read the source without compiling it. Files adder generated are never scanned, and neither are files no platform can build, such as those marked `//go:build ignore`. Platform tags and file name suffixes such as `_windows.go` never exclude a file, so results are the same on every machine. Items are sorted by name. Their fields are listed in [templates](templates.md#scan-items).

### Type-aware scans

With `typeCheck: true`, adder loads the scanned packages' types, so filters compare types rather than how they are written:

```yaml
scans:
  Checks:
    dirs: [checks]
    typeCheck: true
    types: {implements: example.com/app/health.Checker}
```

- `implements` selects every type whose values or pointers implement the interface, written with its import path, or `error`. `.Pointer` is true when only a pointer does. Interfaces and generic types are never selected.
- `funcs.firstParamType` matches through import aliases, dot imports, and type aliases.
- `values.type` matches inferred types too, as in `const Red = Color(1)`.

Type-aware scans are slower, and need the `go` command on `PATH`, code that compiles, and its dependencies downloaded: adder never downloads anything, and reports a missing dependency as a `type-check` problem. Every run checks the same platform, with cgo off and nothing from the environment that changes which files are built. See [`examples/types-implementing-interface`](../examples/types-implementing-interface).

### Directives

A declaration can opt in to a scan with a directive in its doc comment, instead of relying on names:

```go
// GetUser returns one user by ID.
//
//adder:route method=GET path=/users/{id}
func GetUser(w http.ResponseWriter, r *http.Request) {
```

Directives follow Go's convention for tool directives, like `//go:generate`: no space after `//`, then `adder:` and a lowercase name. Arguments are `key=value` pairs, and a value with spaces is written as a Go string. A scan with `directive: route` selects only declarations that have it, and templates read its arguments with `index .Directives "route" "path"`. For an optional directive, `{{ with index .Directives "route" }}...{{ end }}` renders nothing when it is missing. A constant or variable can also take its directive from its group's doc comment. See [`examples/routes-from-directives`](../examples/routes-from-directives).

## Outputs

Each entry under `derive.outputs` renders a `template`, relative to the directory holding `adder.yaml`, to a `path`, relative to the root. Nothing is written unless every output renders.

- **Go outputs** are formatted with gofmt and lose the imports they never use, so templates can import everything a file might need. adder recognizes a package by its explicit name or the last element of its path, so name an import whose package name differs. Missing imports are not added.
- **Every output** is marked with a generated-code header, Go's [standard one](https://go.dev/s/generatedcode) in Go files and the same text in each language's comment syntax elsewhere: `#` for YAML or shell, `--` for SQL, `<!-- -->` for Markdown or HTML, and [so on](design.md#25-generated-file-headers-for-other-languages). JSON and other files without comments get none.
- **adder never replaces a file it did not generate.** When a file without adder's header is at an output's path, `derive` stops with a `file-exists` problem and writes nothing. If another tool generated the file, delete it once; [moving to adder](migrating.md) explains how.
- adder never writes or removes a file through a symbolic link.

### One file per item

An output with `each` renders its template once per item of a scan, as `.Item`, and its `path` is a template too:

```yaml
outputs:
  - template: templates/section.md.tmpl
    each: Sections
    path: "docs/config/{{ .Item.Name | toKebabCase }}.md"
```

Every item must render to a path of its own. Files the output generated before but no longer produces, such as the page of a deleted struct, are removed: `adder check` reports them, and `adder derive` deletes them. A file is only removed when its path matches the output's `path` and its header names the output's template, so the path must end in a file type that has a header. See [`examples/file-per-item`](../examples/file-per-item).

## Generators

Each entry under `generators` is a command, `adder new <name>`, that creates files from templates. Names are lowercase words joined by hyphens.

```yaml
generators:
  command:
    description: Add a command
    inputs:
      name: {help: Command name, pattern: "[a-z][a-z0-9]*", unique: Commands.Package}
      summary: {help: What the command does, default: does nothing yet}
      withTest: {type: bool, help: Add a test file}
    files:
      - template: templates/command.go.tmpl
        path: "commands/{{ .Inputs.name }}/{{ .Inputs.name }}.go"
      - template: templates/command_test.go.tmpl
        path: "commands/{{ .Inputs.name }}/{{ .Inputs.name }}_test.go"
        when: withTest
    message: Write the command in commands/{{ .Inputs.name }}/{{ .Inputs.name }}.go.
```

Input names are lowerCamelCase. Each is a flag in kebab-case, `withTest` is `--with-test`, and a template field, `.Inputs.withTest`. Every problem with the inputs is reported at once, and nothing is written unless the whole run succeeds. In a terminal, `adder new` asks for required inputs that were not given. Scripts, CI, and `--json` get a `missing-input` problem instead.

| Input | Description |
| --- | --- |
| `type` | `string`, the default, or `bool`, which is `false` unless its flag is given. |
| `help` | Shown in `--help` and with problems. |
| `default` | Makes a string input optional. Without one, it is required. |
| `pattern` | A regular expression the whole value must match. |
| `values` | The allowed values. |
| `valuesFrom` | Takes the allowed values from a scan, written `Scan.Field`, such as `Drivers.Package`. |
| `unique` | Rejects values a scan already has, written `Scan.Field`. |
| `deprecated` | Marks the input as on its way out, saying what to use instead. A deprecated string input needs a `default`. |

| File | Description |
| --- | --- |
| `template` | The template, relative to the directory holding `adder.yaml`. |
| `path` | Where to create the file. It is a template, rendered with the same data. |
| `when`, `unless` | A bool input that includes or excludes the file. |
| `ifExists` | `fail`, the default, or `skip` for a file that may already exist, such as a shared README. adder never overwrites a file. |

| Generator | Description |
| --- | --- |
| `description` | Shown in `adder new`. |
| `message` | A template printed after the files are created, such as next steps. |
| `derive` | `false` skips updating derived outputs. |
| `deprecated` | Marks the generator as on its way out, saying what to use instead, such as `use adder new service`. |
| `tests` | See [below](#testing-generators). |

A deprecated generator or input keeps working. `adder new` warns when it runs one, or is given one, and `--help`, `adder list`, and `adder describe` mark them:

```text
adder.yaml:3:3: warning: generator "driver" is deprecated: use adder new store [deprecated]
```

Generator templates see `.Inputs`, `.Vars`, `.Module`, and every scan. Scans see the new files before they are written, so derived outputs include them in the same run. Go files are formatted but get no header, since people own them. See [`examples/scaffold-with-generator`](../examples/scaffold-with-generator).

### Testing generators

A generator's tests render it with sample inputs and compare the result with golden files, at their paths in the repository:

```yaml
    tests:
      - {name: plain, inputs: {name: example}, want: testdata/command/plain}
      - {name: with-test, inputs: {name: example, withTest: true}, want: testdata/command/with-test}
```

`adder test` fails with a diff and a `golden-mismatch` problem when a file is missing, extra, or different. `adder test --update` rewrites the golden files after an intended change. Keep `want` under `testdata`, which scans and Go tooling skip. Tests run against the real repository, so use inputs that `unique` will not reject, such as `example`. Derived outputs are not compared.

## Vars and partials

`vars` holds strings every template reads as `.Vars.<name>`. `partials` lists templates, by [`path.Match`](https://pkg.go.dev/path#Match) patterns, whose `{{ define }}` blocks every template can use:

```yaml
vars:
  License: Apache-2.0
partials:
  - templates/partials/*.tmpl
```

```gotemplate
{{/* templates/partials/license.tmpl */}}
{{ define "license" }}// Licensed under {{ .Vars.License }}.{{ end }}
```

A template uses a partial with `{{ template "license" . }}`, passing the data it needs. Text outside `{{ define }}` blocks is ignored, two partials cannot define the same name, and a template can redefine a block for itself.
