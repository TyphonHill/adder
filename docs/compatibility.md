# Compatibility

From v1.0, adder follows [semantic versioning](https://semver.org/). A configuration, template, pack, or build of adder that works with v1.x keeps working, with the same output, with every later v1 release. Breaking changes wait for a new schema `version` in `adder.yaml`, or v2 of the Go module.

## What is covered

| Area | Promise |
| --- | --- |
| `adder.yaml` and `pack.yaml` | Every key, default, and validation rule for `version: 1`. New keys may be added; a configuration using one needs the release that added it. |
| Template data | The top-level fields `.Module`, `.Vars`, `.Inputs`, `.Options`, `.Item`, and the scans, whose names those fields keep reserved. No new top-level fields are added within v1, so no scan name can become reserved. Scan item fields may be added, and are never removed or renamed. |
| Item order | Every built-in scan sorts its items by name, then file, then receiver. |
| Template functions | Names, arguments, and results, including the case rules and the frozen list of initialisms. New functions may be added. |
| Generated-code header | Its text, where it is written, and the file types that get one. A file type that gets no header today never starts to get one within v1. |
| Commands and flags | Names, arguments, and meaning. New commands and flags may be added. |
| Problems and exit codes | Every diagnostic code keeps its meaning, and the exit codes keep theirs. New codes may be added. |
| JSON output | Every field keeps its name and meaning. New fields may be added, so read JSON in a way that ignores fields it does not know. |
| Go API | The root package, `typhonhill.dev/adder`: `Main`, `Pack`, `Scanner`, `ScanInput`, and `Problem`. Fields may be added to the structs, so write them with field names, as in `adder.Pack{Name: "local"}`. |

## What is not covered

- The wording of messages and hints, and the human-readable output of commands, which may improve in any release. Scripts should read `--json` and match on codes.
- Formatting by gofmt. adder formats Go outputs with the `go/format` package of the Go release it was built with, so a new Go release can change formatting slightly, as it would for `gofmt` itself.
- Bugs. A fix that changes what adder writes is listed under **Fixed** in the [changelog](../CHANGELOG.md), and `adder check` reports the outputs it changes.
- Packages under `internal/`, which no other module can import.

Two additions can still affect an existing setup:

- A new built-in function can share a name with a [function from a pack](extending.md#functions), which is an `invalid-pack` problem. Give your own functions a name a built-in function is unlikely to take, such as one with a prefix: `acmeRouteName` rather than `routeName`.
- A new setting or diagnostic code is unknown to earlier releases, so a configuration that uses it fails with `unknown-field` on them. Pin adder's version in `go.mod`, as `go get -tool` does, so everyone runs the same release.

## Deprecation

Within v1, nothing is removed. A setting, value, default, field, function, command, flag, or code that is replaced is deprecated instead:

1. It keeps working as before.
2. adder warns wherever it is used, at its place, and says what to use instead. The command still succeeds, with the same exit code.
3. The [changelog](../CHANGELOG.md) lists it under **Deprecated**, with what replaces it, and the documentation marks it.
4. It is removed only with a new schema `version`, or in v2 of the Go module. `adder migrate` arrives with the first schema version that needs it, and upgrades a configuration from the version before.

```text
adder.yaml:7:12: warning: derive.scans.Drivers.funcs.name is deprecated: use funcs.match [deprecated]
```

Diagnostic codes, exit codes, and JSON fields are read by scripts rather than used in a configuration, so nothing can warn about them: the changelog and the documentation say when one is deprecated, and a renamed JSON field is written under both names until v2.

Repositories and packs deprecate what they define in the same way, so that their users are warned too: generators, inputs, and pack options with [`deprecated`](configuration.md#generators), and a pack's functions and scanners with [`DeprecatedFuncs` and `DeprecatedScanners`](extending.md#deprecating-what-a-pack-defines) on `adder.Pack`.

## Go versions

adder supports the two most recent Go releases, as Go itself does. When a Go release comes out, the oldest one is dropped, and adder's `go` line may move to the oldest it supports. That is not a breaking change, since Go no longer supports the release that was dropped.
