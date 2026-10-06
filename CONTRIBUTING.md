# Contributing

## Contents

- [Development](#development)
- [Gate](#gate)
- [Logging](#logging)
- [Version](#version)
- [Commits and pull requests](#commits-and-pull-requests)
- [Developer Certificate of Origin](#developer-certificate-of-origin)
- [Reporting bugs](#reporting-bugs)

## Development

Use the Go version in `go.mod`. Run `mise install` for the tool versions in `mise.toml`, and install `cppcheck` with the system package manager. `just fmt` formats Go, C, and Markdown. Do not add SPDX headers. `REUSE.toml` records the MIT license for the tree. Run `just check` before a pull request.

## Gate

`just check` runs the formatters, linters and tests that `mise.toml` pins, and `cppcheck` from the system package manager. `just --show check-go` (or another recipe name) prints the exact commands. `go-test-coverage` fails a non-main package whose statement coverage is below the floor in `.testcoverage.yml`. `mise install` installs the pinned tool versions.

## Logging

Progress goes to stderr through `charm.land/log` as the `log/slog` handler; stdout carries the command result, including under `--json`. The command line does not use `charm.land/fang`: it turns every error into exit status 1, which would drop the `check` status 2.

## Version

`just build` sets `cmd.buildVersion` to the output of `git describe --tags --always --dirty`. A build that does not set it prints the module version from the build information, or `(devel)` when that is empty.

## Commits and pull requests

Each commit contains one logical change and a [Conventional Commits](https://www.conventionalcommits.org/) header. The pull request states what changed and why.

## Developer Certificate of Origin

Every commit requires a `Signed-off-by` trailer. See the [Developer Certificate of Origin](https://developercertificate.org/). Add the trailer with `git commit --signoff`.

## Reporting bugs

Open an issue with the command, the observed result, and the expected result. For a security report, follow [SECURITY.md](SECURITY.md).
