# Scaffold with a generator

This example shows how a generator creates new files from templates with `adder new`, so that every new package in a repository starts the same way. The generator's inputs are validated before anything is written, an optional file is created only when asked for, and tests with golden files keep the templates from changing by accident.

The repository is a set of commands, one package each under `commands/`, and the `command` generator adds one.

## How it works

[`adder.yaml`](adder.yaml) declares the `command` generator:

- `name` is required, must match `[a-z][a-z0-9]*`, and must be `unique` among the packages the `Commands` scan finds, so a command that exists already is refused.
- `summary` is optional, with a default.
- `withTest` is a `bool` input, so it is the flag `--with-test`. The test file is created only `when: withTest`.
- `message` is printed after the files are created, to say what to do next.

The templates are in [`templates/`](templates), and each input is available as `.Inputs.<name>`.

## Try it

Run the commands from this directory, with `go run ../../cmd/adder` in place of `adder` in a checkout of adder. `adder describe` lists what a generator takes:

```console
$ adder describe command
command: Add a command

inputs
  --name       string  Command name, used as the package name (required; must match [a-z][a-z0-9]*; not one of: hello)
  --summary    string  What the command does, for its documentation (default "does nothing yet")
  --with-test  bool    Add a test file
...
```

Invalid inputs are refused before any file is written:

```console
$ adder new command --name hello
adder: --name "hello" already exists in Commands.Package [invalid-input]
	hint: Command name, used as the package name
$ adder new command --name Report
adder: --name "Report" does not match [a-z][a-z0-9]* [invalid-input]
	hint: Command name, used as the package name
```

`--dry-run` shows what a run would create, and the run itself creates it:

```console
$ adder new command --name report --summary "prints a weekly report" --with-test --dry-run
would be created    commands/report/report.go
would be created    commands/report/report_test.go
$ adder new command --name report --summary "prints a weekly report" --with-test
created    commands/report/report.go
created    commands/report/report_test.go

Write the command in commands/report/report.go.
```

In a terminal, `adder new command` asks for any input missing from its flags.

## Testing the generator

The generator's `tests` render it with fixed inputs and compare the result with the files under [`testdata/command`](testdata/command): `plain` without the test file, and `with-test` with it. `adder test` fails when a template change alters what the generator creates, and `adder test --update` rewrites the golden files after an intended change.

```console
$ adder test
ok    command/plain
ok    command/with-test
```

To keep a list of the commands in sync as well, add a derived output, as in [`registry-from-functions`](../registry-from-functions). `adder new` updates derived outputs in the same run.
