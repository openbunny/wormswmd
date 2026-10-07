# Agents

The module path is `github.com/openbunny/wormswmd`.

`just check` is the gate. `mise.toml` pins its tools; the justfile only invokes them.

Format Go with `golangci-lint fmt` (gofmt) rather than `gofumpt`.

`internal/agl/stub/agl_stub.c` is the interop boundary. Qt is not vendored.

Tests use `game.Scaffold` and do not open a real Worms app.

`log/slog` writes operational logs while stdout is the command result.

## CLI conventions

These match `github.com/openbunny/tickerbox-cli`; keep the two CLIs consistent.

`main` runs the root command through `charm.land/fang/v2`, which styles help and
errors, adds `--version`, and registers completions and manpages. `New()` returns
the bare cobra command so tests drive it without fang.

`--json` carries the short `-j`, and JSON output is indented two spaces.

An `ExitError` whose `Err` is nil carries an exit code for a command that already
printed its result; `renderError` leaves it unprinted so fang styles only real
errors.
