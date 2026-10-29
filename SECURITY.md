# Security policy

## Supported versions

adder has not reached v1.0. Security fixes go into the latest release only.

## What counts as a vulnerability

adder promises that running it on a repository is safe, even an untrusted one. Reports that break these guarantees are vulnerabilities:

- Writing or reading files outside the repository root or the configuration file's directory.
- Executing code from the repository or its configuration.
- Making network requests.

Bugs in generated output, crashes, and wrong diagnostics are ordinary bugs; please open an issue for those.

## Reporting a vulnerability

Please do not open a public issue. Report it privately through [GitHub's private vulnerability reporting](https://github.com/TyphonHill/adder/security/advisories/new), and include:

- What you ran, including the output of `adder --version`.
- A configuration and repository layout that reproduce the problem.
- What you expected to happen and what happened instead.

Reports are handled on a best-effort basis, with an acknowledgement within one week. Once a fix is released, the advisory is published with credit to the reporter, unless they prefer otherwise.
