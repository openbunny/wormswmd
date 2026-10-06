package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openbunny/wormswmd/internal/backup"
)

func newRestore() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore Worms W.M.D from an app backup; --backup is required.",
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

func restoredLine(app, dir string) string {
	return fmt.Sprintf("Restored %s from the backup %s.", app, dir)
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
	app, err = backup.Restore(cmd.Context(), dir, app, force)
	if err != nil {
		return failure(err)
	}
	return emit(cmd, map[string]string{"backup": dir, "app": app}, func() error {
		return writeLines(cmd, []string{restoredLine(app, dir)})
	})
}
