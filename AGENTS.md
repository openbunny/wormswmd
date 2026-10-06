# Agents

The module path is `github.com/openbunny/wormswmd`.

`just check` is the gate. `mise.toml` pins its tools; the justfile only invokes them.

Format Go with `golangci-lint fmt` (gofmt) rather than `gofumpt`.

`internal/agl/stub/agl_stub.c` is the interop boundary. Qt is not vendored.

Tests use `game.Scaffold` and do not open a real Worms app.

`log/slog` writes operational logs while stdout is the command result.
