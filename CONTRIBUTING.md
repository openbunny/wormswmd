# Contributing

## Contents

- [Development](#development)
- [Commits and pull requests](#commits-and-pull-requests)
- [Developer Certificate of Origin](#developer-certificate-of-origin)
- [Reporting bugs](#reporting-bugs)

## Development

Use the Go version in `go.mod`. Run `mise install` for the tool versions in `mise.toml`, and install `cppcheck` with the system package manager. `just fmt` formats Go, C, and Markdown. Do not add SPDX headers. `REUSE.toml` records the MIT license for the tree. Run `just check` before a pull request.

## Commits and pull requests

Each commit contains one logical change and a [Conventional Commits](https://www.conventionalcommits.org/) header. The pull request states what changed and why.

## Developer Certificate of Origin

Every commit requires a `Signed-off-by` trailer. See the [Developer Certificate of Origin](https://developercertificate.org/). Add the trailer with `git commit --signoff`.

## Reporting bugs

Open an issue with the command, the observed result, and the expected result. For a security report, follow [SECURITY.md](SECURITY.md).
