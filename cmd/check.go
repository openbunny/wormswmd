package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/openbunny/wormswmd/internal/check"
)

func newCheck() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Report whether Worms W.M.D can open.",
		Args:  cobra.NoArgs,
		RunE:  runCheck,
	}
	cmd.Flags().String("app", "", "Path to Worms W.M.D.app.")
	return cmd
}

func runCheck(cmd *cobra.Command, _ []string) error {
	app, err := flagString(cmd, "app")
	if err != nil {
		return err
	}
	home, applications, err := locations(cmd)
	if err != nil {
		return err
	}
	var probes check.Probes
	slog.Info("checking whether the app can open", "app", app)
	report, err := check.Evaluate(cmd.Context(), app, home, applications, probes)
	if err != nil {
		return failure(err)
	}
	if err := writeCheck(cmd, report); err != nil {
		return err
	}
	slog.Info("check finished", "app", report.App, "ready", report.Ready, "exit", report.Exit)
	if report.Exit == 0 {
		return nil
	}
	return &ExitError{Code: report.Exit}
}

func writeCheck(cmd *cobra.Command, report check.Report) error {
	return emit(cmd, report, func() error {
		headline := "Ready: Worms W.M.D can open."
		if !report.Ready {
			headline = fmt.Sprintf("Not ready: Worms W.M.D will not open (exit %d).", report.Exit)
		}
		var lines []string
		if report.App != "" {
			lines = append(lines, "  App: "+report.App)
		}
		return writeLines(cmd, append(append([]string{headline}, lines...), checkDetails(report)...))
	})
}

func checkDetails(report check.Report) []string {
	return append(bullets("Problems", report.Problems), bullets("Notes", report.Notes)...)
}
