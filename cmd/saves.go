package cmd

import (
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"github.com/openbunny/wormswmd/internal/saves"
)

func newSaves() *cobra.Command {
	parent := &cobra.Command{
		Use:   "saves",
		Short: "Save backup commands.",
	}
	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Back up Worms W.M.D saves.",
		Args:  cobra.NoArgs,
		RunE:  runSavesBackup,
	}
	restoreCmd := &cobra.Command{
		Use:   "restore",
		Short: "Save the current saves to a new backup, then restore Worms W.M.D saves from a directory.",
		Args:  cobra.NoArgs,
		RunE:  runSavesRestore,
	}
	restoreCmd.Flags().String("dir", "", "Save backup directory.")
	markRequired(restoreCmd, "dir")
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List save backup directories.",
		Args:  cobra.NoArgs,
		RunE:  runSavesList,
	}
	parent.AddCommand(backupCmd, restoreCmd, listCmd)
	return parent
}

func runSavesBackup(cmd *cobra.Command, _ []string) error {
	home, err := homeDir(cmd)
	if err != nil {
		return err
	}
	dir, err := saves.Backup(cmd.Context(), home, time.Now())
	if err != nil {
		return failure(err)
	}
	return emit(cmd, dir, func() error { return writeLines(cmd, []string{dir}) })
}

func runSavesRestore(cmd *cobra.Command, _ []string) error {
	home, err := homeDir(cmd)
	if err != nil {
		return err
	}
	dir, err := flagString(cmd, "dir")
	if err != nil {
		return err
	}
	prior, err := saves.Restore(cmd.Context(), home, dir, time.Now())
	if err != nil {
		return failure(err)
	}
	return emit(cmd, dir, func() error {
		lines := []string{}
		if prior != "" {
			lines = append(lines, "Saved the current saves to "+prior)
		}
		return writeLines(cmd, append(lines, "Restored the saves from "+dir))
	})
}

func runSavesList(cmd *cobra.Command, _ []string) error {
	home, err := homeDir(cmd)
	if err != nil {
		return err
	}
	dirs, err := saves.List(cmd.Context(), home)
	if err != nil {
		return failure(err)
	}
	slog.Debug("listed save backups", "count", len(dirs))
	return emit(cmd, dirs, func() error { return writeLines(cmd, dirs) })
}
