# wormswmd

`wormswmd` is a command-line tool for a Worms W.M.D app. The module path is `github.com/openbunny/wormswmd`.

## Table of contents

- [What the tool does](#what-the-tool-does)
- [Requirements](#requirements)
- [Build](#build)
- [Commands](#commands)
- [Logging](#logging)
- [Network](#network)
- [Gate](#gate)
- [License](#license)

## What the tool does

AGL is Apple's OpenGL framework. macOS 26 does not ship its binary. Qt is the UI toolkit inside the Worms W.M.D app. A Mach-O install name is the library path stored in a macOS binary.

`wormswmd apply` backs up the app, builds an AGL stub, and replaces the bundled Qt frameworks from the archive pinned by `PinSHA256` in `internal/qt`. It rewrites Mach-O install names that load `/System/Library/Frameworks/AGL.framework/`, updates `Info.plist`, rewrites known HTTP config URLs, ad-hoc signs the app, clears the quarantine attribute, and deletes the Qt window keys.

`wormswmd fix` runs those writes in `apply`'s order. When `--qt`, `--qt-prefix`, and `WORMSWMD_QT` are unset, `fix` checks the user cache after the host check and before the backup. It downloads the pinned archive when that file is absent or its SHA-256 does not match the pin. When the app already matches a ready check, `fix` does not download and does not write. An error from `apply` exits 1 and does not run `check`. When `apply` returns, `fix` runs `check` and exits with the check status. Text output starts with one sentence that states whether the app is ready to open, then lists the app path and each backup, change, warning, problem and note that is present. JSON output is one object with `apply` and `check` fields. Progress logs go to stderr; by default only warnings and errors print, and `--verbose` prints every step.

Without `--force`, `wormswmd apply` writes nothing when `wormswmd check` would exit 0. `check` exits 0 when the app is ready, 1 when the app is missing or the check returns an error, and 2 when the app is not ready. Exit 2 is any of these:

- `AGL.framework` has no binary.
- QtCore is not version 5.15.
- A regular file under `Contents/Frameworks` or `Contents/PlugIns` contains `/System/Library/Frameworks/AGL.framework/`.
- `Contents/Info.plist` is absent, or lacks `CFBundleIdentifier`, `NSHighResolutionCapable`, `NSSupportsAutomaticGraphicsSwitching`, or `LSMinimumSystemVersion`.
- `codesign --verify --deep --strict` fails.
- The machine is arm64 and Rosetta is absent.
- `QtSystem_GameWindow.geometry` or `QtSystem_GameWindow.windowState` remains in the defaults domain `com.team17.Worms W.M.D`.

A missing `clang` and a present `com.apple.quarantine` attribute are notes. A note does not change the exit code.

The app backup directory is `~/Documents/WormsWMD-Backup-YYYYMMDD-HHMMSS` in UTC. `--backup-dir` is rejected when that path is inside the app. `wormswmd saves backup` writes `~/Documents/WormsWMD-SaveBackups/WormsWMD-SaveBackup-YYYYMMDD-HHMMSS` in UTC. `wormswmd saves list` prints the full path of each directory. `wormswmd saves restore` returns an error when the backup has no `Team17` directory and no Steam id directory.

## Requirements

- macOS on Apple silicon or Intel. Apple silicon needs Rosetta, because the game is an x86_64 app; `apply --install-rosetta` installs it.
- The Xcode Command Line Tools (`xcode-select --install`). `apply` runs `clang`, `otool`, `install_name_tool` and `codesign` from them.
- Go at the version in `go.mod`, to build the binary.

## Build

```console
just build
```

`just build` writes the `wormswmd` binary in the working directory and sets its version to the output of `git describe --tags --always --dirty`. Without `just`, `go build -o wormswmd .` writes the same binary without that version. A build that does not set `cmd.buildVersion` prints the module version, or `(devel)` when the module version is empty.

```console
./wormswmd fix
```

`fix` looks for the game in the Steam library, `/Applications`, `~/Applications`, `~/Games` and the GOG install folders; `--app` names the app when it is elsewhere.

## Commands

- `wormswmd fix` downloads the pinned Qt archive when needed, writes the changes, and exits with the `check` status.
- `wormswmd apply` writes the changes to Worms W.M.D.
- `wormswmd preview` prints the changes and does not write them.
- `wormswmd check` exits 0, 1, or 2 as stated under [What the tool does](#what-the-tool-does).
- `wormswmd restore` restores Worms W.M.D from an app backup.
- `wormswmd support` writes a support report.
- `wormswmd launch` opens Worms W.M.D.
- `wormswmd qt fetch` downloads the archive checked against `PinSHA256` in `internal/qt`.
- `wormswmd saves backup` backs up Worms W.M.D saves.
- `wormswmd saves restore` restores Worms W.M.D saves from a directory.
- `wormswmd saves list` prints the full path of each save backup directory.
- `wormswmd version` prints the version stamped by `just build`.

## Logging

Operational logs go to stderr through `charm.land/log` as the `log/slog` handler. stdout remains the command result, including under `--json`. The command line does not use `charm.land/fang`: it turns every error into exit status 1, which would drop the check status 2.

## Network

`wormswmd qt fetch` downloads the pinned Qt archive from the `dist/` directory of <https://github.com/cboyd0319/WormsWMD-macOS-Fix> at the commit `DistCommit` in `internal/qt` names. `wormswmd fix` downloads that archive when `--qt`, `--qt-prefix`, and `WORMSWMD_QT` are unset and the cached file is absent or its SHA-256 does not match the pin. `wormswmd apply` reads a local archive and does not download.

## Gate

`just check` runs the formatters, linters and tests that `mise.toml` pins, and `cppcheck` from the system package manager. `just --show check-go` (or another recipe name) prints the exact commands. `go-test-coverage` fails a non-main package whose statement coverage is below the floor in `.testcoverage.yml`. `mise install` installs the tool versions pinned in `mise.toml`; `cppcheck` comes from the system package manager.

## License

The license is MIT. The text is [LICENSE](LICENSE).
