package cmd

import (
	"io"
	"log/slog"

	"charm.land/lipgloss/v2"
	charmlog "charm.land/log/v2"
)

type Verbosity int

const (
	Quiet Verbosity = iota
	Normal
	Verbose
)

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
		panic("cmd: unhandled verbosity")
	}
	logger := charmlog.NewWithOptions(w, opts)
	logger.SetStyles(styles)
	slog.SetDefault(slog.New(logger))
}
