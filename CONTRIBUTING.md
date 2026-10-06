# Contributing

## Contents

- [Development](#development)
- [Gate](#gate)
- [Logging](#logging)
- [Version](#version)
- [Releasing](#releasing)
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

## Releasing

A release is a signed tag. Pushing a tag that matches `v*` starts `.github/workflows/release.yml`.

1. Merge the changes to `main` and confirm `just check` passes.
2. Create a signed tag: `git-bot tag -s vX.Y.Z -m "vX.Y.Z"`. Agents use `git-bot`; the owner uses `git tag -s`.
3. Push the tag: `git push origin vX.Y.Z`.

The workflow:

1. Re-runs `ci.yml`, `security.yml`, `gitleaks.yml` and `reuse.yml` through `workflow_call`.
2. Starts the release job only when all four pass. The job runs `goreleaser release --clean` with `GITHUB_TOKEN`, configured in `.goreleaser.yaml`.
3. Builds a `darwin/arm64` binary with `CGO_ENABLED=0` and `-trimpath`, and stamps `cmd.buildVersion` with the tag.
4. Packs the binary with `LICENSE`, `NOTICE` and `LICENSES/*` in `wormswmd_VERSION_darwin_arm64.tar.gz`, writes a CycloneDX SBOM for the archive with `syft`, and writes `checksums.txt`.
5. Signs `checksums.txt` with keyless `cosign` through GitHub Actions OIDC, which produces `checksums.txt.bundle`. See [SECURITY.md](SECURITY.md#release-artifacts).
6. Publishes the GitHub release for the tag.

The Homebrew cask lives in [`OA/homebrew-tap`](https://github.com/OA/homebrew-tap) as `Casks/wormswmd.rb`. Renovate in that repository reads each new release and its `darwin_arm64` archive digest from the GitHub API and opens a pull request that updates the cask version and SHA-256.

On a pull request, the workflow runs `goreleaser release --snapshot --clean --skip=sign`. It builds everything except the signature and publishes nothing.

GoReleaser changes only the release for the pushed tag. The Qt archive stays an asset of the `v0.1.0` release; see [Network](README.md#network).

To run the snapshot locally: `mise exec -- goreleaser release --snapshot --clean --skip=sign`. The output is in `dist/`.

## Commits and pull requests

Each commit contains one logical change and a [Conventional Commits](https://www.conventionalcommits.org/) header. The pull request states what changed and why.

## Developer Certificate of Origin

Every commit requires a `Signed-off-by` trailer. See the [Developer Certificate of Origin](https://developercertificate.org/). Add the trailer with `git commit --signoff`.

## Reporting bugs

Open an issue with the command, the observed result, and the expected result. For a security report, follow [SECURITY.md](SECURITY.md).
