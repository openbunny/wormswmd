package cmd

import (
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/openbunny/wormswmd/internal/support"
)

func newSupport() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "support",
		Short: "Write a support report.",
		Args:  cobra.NoArgs,
		RunE:  runSupport,
	}
	flags := cmd.Flags()
	flags.String("output", "", "Path of the support report.")
	flags.String("app", "", "Path to Worms W.M.D.app.")
	markRequired(cmd, "output")
	return cmd
}

func runSupport(cmd *cobra.Command, _ []string) error {
	output, err := flagString(cmd, "output")
	if err != nil {
		return err
	}
	app, err := flagString(cmd, "app")
	if err != nil {
		return err
	}
	home, applications, err := locations(cmd)
	if err != nil {
		return err
	}
	slog.Info("writing the support report", "output", output)
	if err := support.Write(cmd.Context(), app, home, applications, output); err != nil {
		return failure(err)
	}
	slog.Info("support report written", "output", output)
	return emit(cmd, supportJSON{Output: output}, func() error { return writeLines(cmd, []string{output}) })
}

type supportJSON struct {
	Output string `json:"output"`
}
