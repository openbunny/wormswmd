package cmd

import (
	"github.com/spf13/cobra"

	"github.com/openbunny/wormswmd/internal/support"
)

func newSupport() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "support",
		Short: "Write a support report as a tar archive.",
		Args:  cobra.NoArgs,
		RunE:  runSupport,
	}
	flags := cmd.Flags()
	flags.String("output", "", "Path of the support report. The file is a tar archive that holds report.txt; name it with a .tar extension.")
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
	if err := support.Write(cmd.Context(), app, home, applications, output, support.Env{Version: wormswmdVersion()}); err != nil {
		return failure(err)
	}
	return emit(cmd, supportJSON{Output: output}, func() error { return writeLines(cmd, []string{output}) })
}

type supportJSON struct {
	Output string `json:"output"`
}
