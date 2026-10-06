# Security

Report a vulnerability through a private GitHub Security Advisory at <https://github.com/openbunny/wormswmd/security/advisories/new>. Do not open a public issue for it.

Include the command, the error text, and the version line from `wormswmd version`. Omit save files, config bodies, and game binaries.

## Supported versions

The latest commit on `main` is the only supported version. This file makes no response-time commitment.

## What the tool writes

[README.md](README.md#what-it-changes) lists every change that `fix` and `apply` make to the game app, and [Undo](README.md#undo) describes the restore commands.

## Trust in the Qt archive

The Qt archive is built and published by the author of the upstream WormsWMD-macOS-Fix. This project does not rebuild it. The default, `WORMSWMD_QT` and `--qt` paths check the archive against the SHA-256 compiled into `wormswmd` before extraction. Two inputs bypass that check: `--qt-prefix` copies an extracted Qt directory without any checksum, and `--qt-sha256` replaces the compiled-in checksum (and is accepted only with `--qt` or `WORMSWMD_QT`). Both print a warning. Code from these inputs runs inside the game and the user is responsible for it.
