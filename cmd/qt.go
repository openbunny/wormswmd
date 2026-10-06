package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/openbunny/wormswmd/internal/qt"
)

func newQt() *cobra.Command {
	qtCmd := &cobra.Command{
		Use:   "qt",
		Short: "Qt 5.15 archive commands.",
	}
	fetch := &cobra.Command{
		Use:   "fetch",
		Short: "Download the pinned Qt 5.15 archive.",
		Args:  cobra.NoArgs,
		RunE:  runQtFetch,
	}
	fetch.Flags().String("output", "", "Path of the Qt archive.")
	qtCmd.AddCommand(fetch)
	return qtCmd
}

func qtFetchPath(output, cache string) string {
	if output != "" {
		return output
	}
	return filepath.Join(cache, "wormswmd", qt.ArchiveName)
}

func runQtFetch(cmd *cobra.Command, _ []string) error {
	output, err := flagString(cmd, "output")
	if err != nil {
		return err
	}
	cache := ""
	if output == "" {
		cache, err = os.UserCacheDir()
		if err != nil {
			return failure(fmt.Errorf("user cache directory: %w; pass --output", err))
		}
	}
	dest := qtFetchPath(output, cache)
	slog.Info("fetching the pinned Qt archive", "path", dest)
	if err := qt.Fetch(cmd.Context(), dest, qt.ArchiveURL, qt.PinSHA256); err != nil {
		return failure(err)
	}
	slog.Info("pinned Qt archive is ready", "path", dest)
	return emit(cmd, dest, func() error { return writeLines(cmd, []string{dest}) })
}
