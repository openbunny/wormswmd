# clilog

Operational logging for Go CLI tools. `log/slog` backed by `charm.land/log/v2`,
styled for the terminal and written to stderr, while stdout stays the command
result.

```go
clilog.UseLogger(os.Stderr, clilog.Normal)
```

For a cobra root command, `Attach` declares `--verbose` and `--quiet` and
configures the logger from the command's error stream:

```go
clilog.Attach(root)
```

`--verbose` logs every step with timestamps. `--quiet` drops everything below
the warning level. The two flags cannot be combined.
