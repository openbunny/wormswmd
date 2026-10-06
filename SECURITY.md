# Security

Report a vulnerability through a private GitHub Security Advisory at <https://github.com/openbunny/wormswmd/security/advisories/new>. Do not open a public issue for it.

Include the command, the error text, and the version line from `wormswmd version`. Omit save files, config bodies, and game binaries.

## What apply writes

`wormswmd fix` performs the same writes as `apply`. When `--qt`, `--qt-prefix`, and `WORMSWMD_QT` are unset, it downloads the pinned Qt archive if the cached file is absent or its SHA-256 does not match the pin.

`wormswmd apply` replaces frameworks under `Contents/Frameworks` with the frameworks from the pinned Qt archive. It copies `Contents/PlugIns/platforms/libqcocoa.dylib` and the libraries in `Contents/PlugIns/imageformats`. Where `Contents/PlugIns/accessible` or `Contents/PlugIns/printsupport` exists, `apply` removes that directory. It installs an AGL stub at `Contents/Frameworks/AGL.framework` with the install name `@executable_path/../Frameworks/AGL.framework/AGL`. It rewrites Mach-O load commands that name `/System/Library/Frameworks/AGL.framework/`, including other Mach-O files in each framework and every `*.dylib` under `Contents/PlugIns`.

On `Contents/Info.plist`, `apply` sets `CFBundleIdentifier` to `com.team17.wormswmd` when that key is empty, sets `NSHighResolutionCapable` and `NSSupportsAutomaticGraphicsSwitching` to true, and sets `LSMinimumSystemVersion` to `10.13` when that value is empty or `10.8`. In `Contents/Resources/DataOSX` and `Contents/Resources/CommonData`, `apply` rewrites `http://www.team17.com` and `http://www.google-analytics.com` to `https`, and prefixes `// DISABLED:` and one space onto `URL_Internal` lines that contain `xom.team17.com`.

`apply` signs the bundle with `codesign --force --deep --sign -` and removes the `com.apple.quarantine` attribute. `apply` deletes `QtSystem_GameWindow.geometry` and `QtSystem_GameWindow.windowState` from the defaults domain `com.team17.Worms W.M.D`.

The backup directory is `~/Documents/WormsWMD-Backup-YYYYMMDD-HHMMSS` in UTC. A `--backup-dir` path inside the app is rejected. `wormswmd restore` copies that backup back onto the app.
