# Templates

Templates are Go's [`text/template`](https://pkg.go.dev/text/template), with the data and functions below. Referencing a field that does not exist is an error, so a typo fails loudly, with its position and the fields that do exist:

```text
templates/registry.go.tmpl:12:4: <.Drivrs>: map has no entry for key "Drivrs" [template-exec]
	hint: available fields: Drivers, Module, Vars
```

## Data

| Field | In | Description |
| --- | --- | --- |
| `.<Scan>` | every template | The items of each scan, by the scan's name, such as `.Drivers`. |
| `.Module` | every template | The module path from the root's `go.mod`, or `""` without one. |
| `.Vars` | every template but a pack's | The strings in `vars`. |
| `.Item` | outputs with `each` | The item being rendered. |
| `.Inputs` | generators | The inputs, by name, such as `.Inputs.name`. |
| `.Options` | a pack's templates | The options of the use, such as `.Options.out`. |

### Scan items

Every item has these fields. Those that do not apply to it are empty.

| Field | Description |
| --- | --- |
| `.Name` | Declared name, or for file scans the file name. |
| `.Kind` | `func`, `method`, `struct`, `interface`, `other` (another type, such as `type Status int`), `const`, `var`, or `file`. |
| `.Receiver` | Receiver type for methods. |
| `.Package` | Package name from the file's `package` clause. |
| `.ImportPath` | The package's import path, from the nearest `go.mod`. Empty without one. |
| `.Dir`, `.File` | Directory and file, relative to the root. |
| `.Line` | Line of the declaration. |
| `.Doc` | Doc comment, without directives. |
| `.Signature` | For functions, the parameters and results as written, such as `(ctx context.Context) error`. |
| `.Params`, `.Results` | For functions, each parameter and result, with `.Name` and `.Type`. |
| `.Fields` | For structs, each field, with `.Name`, `.Type`, `.Tag`, and `.Embedded`. |
| `.Type`, `.Value` | For constants and variables, the type and value as written. `.Type` repeats through a `const` group. |
| `.Iota` | For constants, the position in their `const` group, which is their value of `iota`. |
| `.Pointer` | For types selected by `implements`, true when only a pointer implements the interface, so a template writes `&{{ .Name }}{}`. |
| `.Directives` | The arguments of each [directive](configuration.md#directives), by directive name. |

Items from [custom scanners](extending.md#scanners) have the fields their scanner gives them.

## Functions

Alongside Go's [built-in functions](https://pkg.go.dev/text/template#hdr-Functions), adder has a small set of its own, all deterministic. Names follow [Sprout](https://github.com/go-sprout/sprout) where it has an equivalent, and like Sprout's, they take the value last so they work at the end of a pipeline.

| Function | Example | Result |
| --- | --- | --- |
| `toLower`, `toUpper` | `toUpper "abc"` | `ABC` |
| `toCamelCase`, `toPascalCase` | `toPascalCase "user_id"` | `UserId` |
| `toGoCamelCase`, `toGoPascalCase` | `toGoPascalCase "user_id"` | `UserID` |
| `toSnakeCase`, `toKebabCase` | `toSnakeCase "HTTPServer"` | `http_server` |
| `toConstantCase` | `toConstantCase "HTTPServer"` | `HTTP_SERVER` |
| `fieldValues` | `fieldValues "Package" .Drivers` | the `Package` of every item |
| `groupBy` | `groupBy "Dir" .Handlers` | groups, each with `.Key` and `.Items`, sorted by key |
| `sortBy` | `sortBy "Iota" .Colors` | the items sorted by a field, keeping the order of equal ones |
| `uniq`, `sortAlpha` | `sortAlpha (fieldValues "Dir" .Handlers)` | strings without repeats, or sorted |
| `join` | `fieldValues "Name" .Drivers \| join ", "` | `disk, memory` |
| `trimPrefix`, `trimSuffix` | `trimPrefix "Handle" .Name` | `Users` for `HandleUsers` |
| `replace` | `replace "-" "_" .Dir` | `go_redis` for `go-redis` |
| `quote` | `quote .Name` | `"s3"`, quoted as Go writes strings |
| `indent`, `nindent` | `nindent 4 .Doc` | each line indented by four spaces, after a newline with `nindent` |
| `plural` | `plural "driver" "drivers" (len .Drivers)` | `driver` for exactly one, `drivers` otherwise |

To add functions of your own, see [extending adder](extending.md#functions).

### Case

Case functions split words at separators such as `_`, `-`, and spaces, at a change from lower to upper case, and at the end of an acronym, so `HTTPServer` is `HTTP` and `Server`.

The Go variants make identifiers in Go's naming style. They keep words already in capitals, such as `BMC`, and capitalize the initialisms in [staticcheck's list](https://staticcheck.dev/docs/configuration/options/#initialisms), such as `ID`, `HTTP`, and `URL`. That list is frozen from v1.0, so upgrading adder never renames a generated identifier. For another initialism, write it in capitals in the input: `user_SDK` is `UserSDK`.

Because capitals are kept, input written entirely in capitals stays that way: `toGoPascalCase "LISTEN_ADDR"` is `LISTENADDR`. Lower it first, `toLower .Key | toGoPascalCase`, to get `ListenAddr`. [`examples/custom-scanner`](../examples/custom-scanner) does this for keys read from an `.env` file.
