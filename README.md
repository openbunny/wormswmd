# wormswmd

Worms W.M.D does not open on macOS 26. Apple removed the AGL binary that the game loads, and the Qt libraries bundled in the game do not work on macOS 26. `wormswmd` changes the installed game app so that it opens. It makes a backup of the app first.

`wormswmd` is unofficial. It is not affiliated with or endorsed by Team17, the Qt Company, Valve or Apple. Worms, Worms W.M.D and Team17 are trademarks of their owners. Qt is a trademark of The Qt Company. Steam is a trademark of Valve Corporation. `wormswmd` contains no game files. The procedure it follows comes from [WormsWMD-macOS-Fix](https://github.com/cboyd0319/WormsWMD-macOS-Fix) by Chad Boyd; see [NOTICE](NOTICE).

## Table of contents

- [Requirements](#requirements)
- [Install](#install)
- [Quick start](#quick-start)
- [What it changes](#what-it-changes)
- [Undo](#undo)
- [Risks](#risks)
- [Troubleshooting](#troubleshooting)
- [Where it looks for the game](#where-it-looks-for-the-game)
- [Commands](#commands)
- [Output and logging](#output-and-logging)
- [Network](#network)
- [Supported versions](#supported-versions)
- [Contributing](#contributing)
- [License](#license)

## Requirements

- macOS 26 or later on Apple silicon or Intel. On Apple silicon the game needs Rosetta, because it is an x86_64 app. `wormswmd fix --install-rosetta` installs Rosetta when it is absent.
- The Xcode Command Line Tools: `xcode-select --install`. `fix` and `apply` run `clang`, `otool`, `install_name_tool` and `codesign` from them, and stop before any change when `clang` is missing.
- Go at the version in `go.mod`, to install or build `wormswmd`. Building with `just build` also needs `just` and `git`.

## Install

```console
go install github.com/openbunny/wormswmd@latest
```

This writes `wormswmd` to the directory `go env GOBIN` prints, or to `$(go env GOPATH)/bin` when that is empty. Add that directory to `PATH`.

To build from a checkout:

```console
go build -o wormswmd .
```

`just build` builds the same binary and stamps it with the output of `git describe --tags --always --dirty`.

## Quick start

1. `wormswmd preview` prints the changes and writes nothing.
2. `wormswmd fix` makes the changes and then checks the result. The first line of its output states whether the game is ready to open. It also prints the app path and the path of the backup.
3. `wormswmd launch` opens the game.

Close the game before step 2. If the game is installed outside the folders listed under [Where it looks for the game](#where-it-looks-for-the-game), add `--app "/path/to/Worms W.M.D.app"` to each command.

## What it changes

`wormswmd fix` and `wormswmd apply` make these changes, in this order:

1. Back up the files that the later steps change, to `~/Documents/WormsWMD-Backup-YYYYMMDD-HHMMSS` (time in UTC). `--backup-dir` names a different directory; it is rejected when it is inside the game app.
2. Build a small replacement for the missing AGL library and install it at `Contents/Frameworks/AGL.framework`.
3. Replace the Qt libraries under `Contents/Frameworks` with the Qt 5.15 libraries from a downloaded archive. Copy `Contents/PlugIns/platforms/libqcocoa.dylib` and the libraries in `Contents/PlugIns/imageformats` from the same archive. Remove `Contents/PlugIns/accessible` and `Contents/PlugIns/printsupport` when they exist.
4. Rewrite every library path of the form `/System/Library/Frameworks/AGL.framework/...` in the game's libraries and plug-ins so that it points at the replacement from step 2.
5. In `Contents/Info.plist`: set `CFBundleIdentifier` to `com.team17.wormswmd` when it is empty, set `NSHighResolutionCapable` and `NSSupportsAutomaticGraphicsSwitching` to true, and set `LSMinimumSystemVersion` to `10.13` when it is empty or `10.8`.
6. In the config files under `Contents/Resources/DataOSX` and `Contents/Resources/CommonData`: change `http://www.team17.com` and `http://www.google-analytics.com` to `https`, and disable each `URL_Internal` line that contains `xom.team17.com` by prefixing it with `// DISABLED:` and one space.
7. Sign the whole app again with an ad-hoc signature (`codesign --force --deep --sign -`). This replaces the original signature.
8. Remove the `com.apple.quarantine` attribute from the app.
9. Delete the saved window position and size of the game (`QtSystem_GameWindow.geometry` and `QtSystem_GameWindow.windowState` in the defaults domain `com.team17.Worms W.M.D`), so that the game opens with a default window.

Saves are not changed.

`fix` downloads the Qt archive first when it needs one; see [Network](#network). When the app already passes `check`, `fix` downloads nothing and writes nothing, and `apply` writes nothing unless `--force` is given. `fix --force` makes the changes even when `check` passes.

If a step after the first change fails, the error names the backup directory and states that the app was modified.

## Undo

Restore the app from the backup that `fix` or `apply` printed:

```console
wormswmd restore --backup ~/Documents/WormsWMD-Backup-YYYYMMDD-HHMMSS
```

`--backup` is required. `restore` copies the backed-up files over the app and prints one line naming the restored app and the backup it came from. It restores app files only. It does not undo the quarantine attribute removal or the deleted window settings. It restores the app path recorded in the backup. `--app` names a different app, and `--force` is required when that path differs from the recorded one.

Back up saves with `wormswmd saves backup`. It writes `~/Documents/WormsWMD-SaveBackups/WormsWMD-SaveBackup-YYYYMMDD-HHMMSS` (time in UTC) and prints the path. `wormswmd saves list` prints every save backup. `wormswmd saves restore --dir PATH` first backs up the current saves to a new save backup and prints its path, then restores the saves from `PATH`. It fails when `PATH` contains neither a `Team17` directory nor a Steam id directory.

## Risks

- The Qt libraries come from a third-party archive. The author of WormsWMD-macOS-Fix built and published it. This project has not rebuilt it from source and does not mirror it. Trust rests on the SHA-256 checksum compiled into `wormswmd`, which rejects a download whose checksum differs. The checksum shows that the file is the one that was pinned; it does not show how the file was built. If that repository removes the file, `fix` cannot download it, and the error says how to supply the archive another way.
- `--qt PATH` and `WORMSWMD_QT` use an archive you supply, checked against the compiled-in checksum. `--qt-sha256` replaces that checksum and is accepted only together with `--qt` or `WORMSWMD_QT`. `--qt-prefix` uses an extracted Qt directory without any checksum check. `wormswmd` warns that the pinned checksum is not used when you pass `--qt-prefix` or `--qt-sha256`. Native code from these inputs runs inside the game; use only builds you trust.
- The game app is modified and signed again with an ad-hoc signature, not the original one.
- A Steam file verification or a game update can replace the changed files. When the game stops opening after either, run `wormswmd fix` again.
- The software comes with no warranty, as the [MIT license](LICENSE) states.

## Troubleshooting

`wormswmd check` reports whether the game can open and exits with one of three statuses:

- 0: ready.
- 1: no app was found, several apps were found, or the check failed with an error. When no app was found, the first line reads `Not found: no Worms W.M.D app was located (exit 1).` When several were found, the first line names `--app`.
- 2: not ready. The first line starts with `Not ready`. Any of these causes it:
  - `AGL.framework` has no binary.
  - QtCore is not version 5.15.
  - A file under `Contents/Frameworks` or `Contents/PlugIns` still names `/System/Library/Frameworks/AGL.framework/`.
  - `Contents/Info.plist` is absent or lacks `CFBundleIdentifier`, `NSHighResolutionCapable`, `NSSupportsAutomaticGraphicsSwitching` or `LSMinimumSystemVersion`.
  - `codesign --verify --deep --strict` fails.
  - The machine is Apple silicon and Rosetta is absent.
  - The saved window position or size remains in the defaults domain `com.team17.Worms W.M.D`.

A missing `clang` and a present quarantine attribute are notes and do not change the status.

`wormswmd fix` exits with the `check` status after the changes. It exits 1 without running `check` when `apply` fails.

To report a problem, write a support report with `wormswmd support --output support.tar`. It contains one text report and is sent nowhere. Read it before you attach it, because it can contain your home directory name; see [PRIVACY.md](PRIVACY.md). Then open an issue with the bug report template at <https://github.com/openbunny/wormswmd/issues/new/choose>.

## Where it looks for the game

When `--app` is absent, the commands look in these locations in this order. `--home` replaces the home directory (`~`) and `--applications` replaces `/Applications`.

1. `~/Library/Application Support/Steam/steamapps/common/WormsWMD/Worms W.M.D.app`
2. `/Applications/Worms W.M.D.app` and `/Applications/Worms WMD.app`
3. `~/Applications/Worms W.M.D.app` and `~/Applications/Worms WMD.app`
4. `~/Games/Worms W.M.D.app` and `~/Games/Worms WMD.app`
5. `~/GOG Games/Worms W.M.D/Worms W.M.D.app` and `~/GOG Games/Worms W.M.D.app`
6. `~/Library/Application Support/GOG.com/Games/Worms W.M.D/Worms W.M.D.app`
7. Any `Worms W.M.D.app` or `Worms WMD.app` up to two folder levels below `/Applications`, `~/Applications`, `~/Games`, `~/GOG Games` and `~/Library/Application Support/GOG.com/Games`.
8. `steamapps/common/WormsWMD/Worms W.M.D.app` in each library listed in `~/Library/Application Support/Steam/steamapps/libraryfolders.vdf`.

Two paths to the same app count once. When no app is found, the command exits 1 and the error says to pass `--app`. When several different apps are found, the command exits 1 and lists them; pass `--app` to choose one.

## Commands

- `wormswmd fix` downloads the Qt archive when needed, makes the changes and exits with the `check` status.
- `wormswmd apply` makes the changes without downloading.
- `wormswmd preview` prints the changes and writes nothing.
- `wormswmd check` reports whether the game can open.
- `wormswmd restore --backup DIR` restores the app from a backup. Flags: `--app`, `--force`.
- `wormswmd launch` opens the game.
- `wormswmd support --output PATH` writes a support report.
- `wormswmd qt fetch` downloads the Qt archive and checks it against the compiled-in checksum. Flag: `--output`; the default is `wormswmd/qt-frameworks-x86_64-5.15.19.tar.gz` in the user cache directory.
- `wormswmd saves backup`, `wormswmd saves list` and `wormswmd saves restore --dir PATH` handle saves.
- `wormswmd version` prints the version.

`fix`, `apply` and `preview` take `--app`, `--qt`, `--qt-prefix`, `--qt-sha256` and `--force`; `fix` and `apply` also take `--backup-dir` and `--install-rosetta`. `preview` installs nothing and stops when Rosetta is absent. `check`, `launch` and `support` take `--app`. `wormswmd COMMAND --help` lists the flags of a command.

## Output and logging

stdout carries the command result and stderr carries progress. `--json` prints the result as JSON on stdout. `--quiet` prints only warnings and errors. `--verbose` adds timestamps and details, including every external command and its duration. `--verbose` and `--quiet` cannot be combined.

## Network

`wormswmd qt fetch` always downloads the Qt archive. `wormswmd fix` downloads it only when `--qt`, `--qt-prefix` and `WORMSWMD_QT` are all unset and the cached archive is absent or does not match the checksum. The source is the `dist/` directory of <https://github.com/cboyd0319/WormsWMD-macOS-Fix>, served from raw.githubusercontent.com. A failed download names the URL, the expected SHA-256 and the remedy: download the archive another way and pass `--qt PATH`. No other command uses the network. See [PRIVACY.md](PRIVACY.md).

## Supported versions

The latest commit on `main` is the only supported version. Security reports follow [SECURITY.md](SECURITY.md).

## Contributing

Build, gate and logging details are in [CONTRIBUTING.md](CONTRIBUTING.md).

## License

`wormswmd` is MIT licensed; the text is in [LICENSE](LICENSE). [NOTICE](NOTICE) names the two third-party items: the MIT-licensed upstream procedure and the LGPL Qt libraries that `fix` downloads.
