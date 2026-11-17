package clilog

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// Attach declares the --verbose and --quiet flags on root and installs a
// PersistentPreRunE that configures the slog logger from the command's error
// stream. An already-installed PersistentPreRunE runs after the logger is set.
func Attach(root *cobra.Command) {
	root.PersistentFlags().Bool("verbose", false, "Log every step with timestamps and details to stderr.")
	root.PersistentFlags().Bool("quiet", false, "Log only warnings and errors to stderr.")
	previous := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		verbosity, err := verbosityOf(cmd)
		if err != nil {
			return err
		}
		UseLogger(cmd.ErrOrStderr(), verbosity)
		if previous != nil {
			return previous(cmd, args)
		}
		return nil
	}
}

func verbosityOf(cmd *cobra.Command) (Verbosity, error) {
	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return 0, fmt.Errorf("flag --verbose: %w", err)
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return 0, fmt.Errorf("flag --quiet: %w", err)
	}
	switch {
	case verbose && quiet:
		return 0, errors.New("--verbose and --quiet cannot be combined; pass one of them")
	case verbose:
		return Verbose, nil
	case quiet:
		return Quiet, nil
	default:
		return Normal, nil
	}
}
