package clilog

import (
	"io"
	"log/slog"

	"charm.land/lipgloss/v2"
	charmlog "charm.land/log/v2"
)

// Verbosity is how much an operational log reports.
type Verbosity int

const (
	// Quiet logs warnings and errors only.
	Quiet Verbosity = iota
	// Normal logs warnings, errors, and human-readable progress.
	Normal
	// Verbose logs every step, with timestamps.
	Verbose
)

// UseLogger installs a slog logger that writes styled operational logs to w.
// stdout is the command result; log lines belong on stderr.
func UseLogger(w io.Writer, verbosity Verbosity) {
	opts := charmlog.Options{Level: charmlog.InfoLevel}
	styles := charmlog.DefaultStyles()
	switch verbosity {
	case Quiet:
		opts.Level = charmlog.WarnLevel
	case Normal:
		styles.Levels[charmlog.InfoLevel] = lipgloss.NewStyle().SetString("›").Bold(true).Foreground(lipgloss.Color("86"))
	case Verbose:
		opts.Level = charmlog.DebugLevel
		opts.ReportTimestamp = true
	default:
		panic("clilog: unhandled verbosity")
	}
	logger := charmlog.NewWithOptions(w, opts)
	logger.SetStyles(styles)
	slog.SetDefault(slog.New(logger))
}
