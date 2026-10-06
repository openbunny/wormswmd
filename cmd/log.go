package cmd

import (
	"io"
	"log/slog"

	charmlog "charm.land/log/v2"
)

func UseLogger(w io.Writer, verbose bool) {
	level := charmlog.WarnLevel
	if verbose {
		level = charmlog.InfoLevel
	}
	slog.SetDefault(slog.New(charmlog.NewWithOptions(w, charmlog.Options{
		ReportTimestamp: true,
		Level:           level,
	})))
}
