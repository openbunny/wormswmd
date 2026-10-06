package cmd

import (
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/openbunny/wormswmd/internal/game"
	"github.com/openbunny/wormswmd/internal/run"
)

var openExec run.Exec = run.Command

func newLaunch() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "launch",
		Short: "Open Worms W.M.D.",
		Args:  cobra.NoArgs,
		RunE:  runLaunch,
	}
	cmd.Flags().String("app", "", "Path to Worms W.M.D.app.")
	return cmd
}

func runLaunch(cmd *cobra.Command, _ []string) error {
	app, err := flagString(cmd, "app")
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	if app == "" {
		home, applications, err := locations(cmd)
		if err != nil {
			return err
		}
		app, err = game.Resolve(ctx, app, home, applications)
		if err != nil {
			return failure(err)
		}
	} else if err := game.Valid(ctx, app); err != nil {
		return failure(err)
	}
	slog.Info("Opening Worms W.M.D")
	slog.Debug("opening the app", "app", app)
	out, err := openExec(ctx, "open", app)
	if err != nil {
		return failure(err)
	}
	return emit(cmd, app, func() error {
		if len(out) == 0 {
			return nil
		}
		_, err := cmd.OutOrStdout().Write(out)
		return failure(err)
	})
}
