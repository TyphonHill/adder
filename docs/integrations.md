# Integrations

adder works alongside the tools a repository already uses. Each snippet below runs `go tool adder`, the version `go.mod` pins, so every machine and CI job runs the same release.

## go generate

One directive anywhere in the module regenerates every output, since adder finds `adder.yaml` from any directory:

```go
//go:generate go tool adder derive
```

Put it in a single file, such as `doc.go` at the root, since `go generate ./...` runs each directive it finds.

## pre-commit

With [pre-commit](https://pre-commit.com), a local hook runs the adder `go.mod` pins, including a repository's [own build](extending.md#your-own-build) with its packs:

```yaml
# .pre-commit-config.yaml
repos:
  - repo: local
    hooks:
      - id: adder-check
        name: adder check
        entry: go tool adder check
        language: system
        pass_filenames: false
        always_run: true
```

Use `adder derive` as the entry instead to update the derived files on commit. pre-commit then fails the commit when one changed, so that it can be added.

A repository that does not declare adder as a tool can use the hooks adder publishes, which build adder from source at the revision given:

```yaml
repos:
  - repo: https://github.com/typhonhilldev/adder
    rev: v1.0.0
    hooks:
      - id: adder-check   # or adder-derive
```

The hooks run on every commit, whatever changed, since a change anywhere can affect a scan. In a [monorepo](commands.md#monorepos) without an `adder.yaml` at its root, give them `args: [./...]`.

## GitHub Actions

`adder check --diff` fails the job when a derived file is out of date, and shows what changed:

```yaml
# .github/workflows/ci.yml
jobs:
  adder:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with: {go-version-file: go.mod}
      - run: go tool adder check --diff
```

To show problems and out-of-date files as annotations on the pull request, read `--json` instead:

```yaml
      - run: |
          go tool adder check --json > adder.json || status=$?
          jq -r '
            [., (.configs // [])[]] | .[]
            | (.config // "" | sub("adder.yaml$"; "")) as $dir
            | ((.problems // [])[] | "::error file=\(.pos.file // ""),line=\(.pos.line // 1),col=\(.pos.column // 1)::\(.message) [\(.code)]"),
              ((.warnings // [])[] | "::warning file=\(.pos.file // ""),line=\(.pos.line // 1),col=\(.pos.column // 1)::\(.message) [\(.code)]"),
              ((.changes // [])[] | select(.status != "unchanged") | "::error file=\($dir + .path)::\($dir + .path) is out of date; run adder derive [stale]")
          ' adder.json
          exit ${status:-0}
```

The same step works with `go tool adder check ./... --json` in a [monorepo](commands.md#monorepos).

## GitLab CI

```yaml
# .gitlab-ci.yml
adder:
  image: golang:1.27
  script:
    - go tool adder check --diff
```

To show problems and out-of-date files in the merge request's [Code Quality](https://docs.gitlab.com/ci/testing/code_quality/) widget, convert `--json` into a Code Quality report:

```yaml
adder:
  image: golang:1.27
  script:
    - apt-get update -qq && apt-get install -y -qq jq
    - go tool adder check --json > adder.json || status=$?
    - |
      jq '[
        [., (.configs // [])[]] | .[]
        | (.config // "" | sub("adder.yaml$"; "")) as $dir
        | ((.problems // [])[] | {severity: "major", code, message, file: (.pos.file // "adder.yaml"), line: (.pos.line // 1)}),
          ((.warnings // [])[] | {severity: "minor", code, message, file: (.pos.file // "adder.yaml"), line: (.pos.line // 1)}),
          ((.changes // [])[] | select(.status != "unchanged") | {severity: "major", code: "stale", message: "\($dir + .path) is out of date; run adder derive", file: ($dir + .path), line: 1})
        | {description: "\(.message) [\(.code)]", check_name: .code, severity, fingerprint: ("\(.code):\(.file):\(.line):\(.message)" | @base64), location: {path: .file, lines: {begin: .line}}}
      ]' adder.json > gl-code-quality-report.json
    - exit ${status:-0}
  artifacts:
    when: always
    reports:
      codequality: gl-code-quality-report.json
```

## Coding agents

See [coding agents](agents.md) for how agents discover and run generators, and an `AGENTS.md` section to copy into a repository.
