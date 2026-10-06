package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/openbunny/wormswmd/internal/backup"
)

func newRestore() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore Worms W.M.D from an app backup.",
		Args:  cobra.NoArgs,
		RunE:  runRestore,
	}
	flags := cmd.Flags()
	flags.String("backup", "", "App backup directory.")
	flags.String("app", "", "Path to Worms W.M.D.app.")
	flags.Bool("force", false, "Replace the current app.")
	markRequired(cmd, "backup")
	return cmd
}

func runRestore(cmd *cobra.Command, _ []string) error {
	dir, err := flagString(cmd, "backup")
	if err != nil {
		return err
	}
	app, err := flagString(cmd, "app")
	if err != nil {
		return err
	}
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return failure(fmt.Errorf("flag --force: %w", err))
	}
	slog.Info("restoring the app", "backup", dir, "app", app)
	if err := backup.Restore(cmd.Context(), dir, app, force); err != nil {
		return failure(err)
	}
	slog.Info("restore finished", "backup", dir, "app", app)
	return emit(cmd, map[string]string{"backup": dir, "app": app}, func() error { return nil })
}
