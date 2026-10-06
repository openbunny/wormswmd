package cmd

import (
	"log/slog"
	"runtime/debug"

	"github.com/spf13/cobra"
)

var buildVersion string

func newVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the wormswmd version.",
		Args:  cobra.NoArgs,
		RunE:  runVersion,
	}
}

type versionJSON struct {
	Version string `json:"version"`
}

func wormswmdVersion() string {
	version := buildVersion
	if version == "" {
		info, ok := debug.ReadBuildInfo()
		if ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}
	if version == "" {
		version = "(devel)"
	}
	return version
}

func runVersion(cmd *cobra.Command, _ []string) error {
	version := wormswmdVersion()
	slog.Debug("resolved the version", "version", version)
	return emit(cmd, versionJSON{Version: version}, func() error { return writeLines(cmd, []string{version}) })
}
