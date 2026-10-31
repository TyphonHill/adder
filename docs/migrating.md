# Moving to adder

Many repositories have a generator of their own: a `go run ./gen` program, or a script behind `//go:generate`, that renders templates into registries and new files. adder replaces it with `adder.yaml` and the same templates. The move is made once, in one change, and everyone else gets it by pulling.

## Map the generator

| Hand-written generator | adder |
| --- | --- |
| Walks directories and parses Go files to build a list | A [scan](configuration.md#scans): `funcs`, `types`, `values`, or `files`, with filters for names, signatures, and directives |
| Renders a list into a file | An [output](configuration.md#outputs), or one with `each` for a file per item |
| A subcommand that creates new files from its flags | A [generator](configuration.md#generators), with validated inputs, as `adder new <name>` |
| A case conversion library | [Case functions](templates.md#case), such as `toGoPascalCase` |
| A hard-coded module path | `{{ .Module }}` |
| A `// Code generated ... DO NOT EDIT.` line in each template | Nothing: adder writes the header itself |
| Logic no scan expresses | A [custom scanner](extending.md#scanners) |

Templates usually move as they are, renamed to what the data is called in adder: `.Inputs.name` for what was a flag, and the scan's name for a list. Inputs are given as arguments, `name=gcs`, rather than flags. Run `adder describe <generator>` to compare a generator's inputs with the old flags.

## Make the change

1. Write `adder.yaml` and the templates, and remove the old generator and its `//go:generate` lines.
2. Delete the files the old generator wrote at the outputs' paths.
3. Run `adder derive`, and compare its output with the old files: apart from the header, they should match.
4. Commit the outputs with the rest of the change, and add `adder check` to CI.

Step 2 is needed because adder never replaces a file it did not generate, even one another tool generated. Without it, `adder derive` stops and says so:

```text
drivers/drivers.gen.go: file already exists and adder did not generate it, so it is not replaced by templates/registry.go.tmpl [file-exists]
	hint: another tool generated this file; delete it once, as when moving to adder, and adder generates it from then on
```

From then on, adder recognizes the files by its own header, and updates and removes them as the code changes.

## Everyone else

The files arrive with the change, so other checkouts need no step of their own. If the old generator's files were ignored by git, as generated files often are, git replaces each checkout's copies when it pulls. If they were neither committed nor ignored, git refuses to pull over them:

```text
error: The following untracked working tree files would be overwritten by merge:
	drivers/drivers.gen.go
```

Delete those files once, and pull again.
