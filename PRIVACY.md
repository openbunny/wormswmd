# Privacy

`wormswmd` sends no paths, file contents or reports over the network. The only network calls download the pinned Qt archive: `wormswmd qt fetch` always, and `wormswmd fix` when `--qt`, `--qt-prefix`, and `WORMSWMD_QT` are unset and the cached file is absent or its SHA-256 does not match the pin.

`wormswmd support` writes a tar archive that contains one text report and sends it nowhere. The report lists the app path, the store the game came from, whether the AGL stub is present, the QtCore version, and the error text of each check that failed. The app path includes the home directory name when the app is inside it, and error text can name other paths. The archive does not contain the game binary, saves, or config bodies. Read the report before sharing it.
