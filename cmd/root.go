package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openbunny/wormswmd/internal/check"
)

var rootLong = fmt.Sprintf(`Prepare Worms W.M.D to open on macOS %d or later.

Examples:
  wormswmd check
  wormswmd preview
  wormswmd fix`, check.MinMajor)

func New() *cobra.Command {
	cobra.EnableCommandSorting = false
	root := &cobra.Command{
		Use:           "wormswmd",
		Short:         "Prepare Worms W.M.D to open on macOS 26.",
		Long:          rootLong,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			verbose, err := cmd.Flags().GetBool("verbose")
			if err != nil {
				return failure(fmt.Errorf("flag --verbose: %w", err))
			}
			quiet, err := cmd.Flags().GetBool("quiet")
			if err != nil {
				return failure(fmt.Errorf("flag --quiet: %w", err))
			}
			switch {
			case verbose && quiet:
				return failure(errors.New("--verbose and --quiet cannot be combined; pass one of them"))
			case verbose:
				UseLogger(cmd.ErrOrStderr(), Verbose)
			case quiet:
				UseLogger(cmd.ErrOrStderr(), Quiet)
			default:
				UseLogger(cmd.ErrOrStderr(), Normal)
			}
			return nil
		},
	}
	root.PersistentFlags().Bool("json", false, "Print JSON.")
	root.PersistentFlags().Bool("verbose", false, "Log every step with timestamps and details to stderr.")
	root.PersistentFlags().Bool("quiet", false, "Log only warnings and errors to stderr.")
	root.PersistentFlags().String("home", "", "Home directory under which the game, the saves and the backups are looked up. An empty value uses the user home directory.")
	root.PersistentFlags().String("applications", "", "Applications directory searched for the game. An empty value uses /Applications.")
	root.AddCommand(
		newFix(),
		newApply(false),
		newApply(true),
		newCheck(),
		newRestore(),
		newSupport(),
		newLaunch(),
		newQt(),
		newSaves(),
		newVersion(),
	)
	return root
}

func emit(cmd *cobra.Command, v any, plain func() error) error {
	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return failure(fmt.Errorf("flag --json: %w", err))
	}
	if asJSON {
		return writeJSON(cmd, v)
	}
	return plain()
}

func flagString(cmd *cobra.Command, name string) (string, error) {
	value, err := cmd.Flags().GetString(name)
	if err != nil {
		return "", failure(fmt.Errorf("flag --%s: %w", name, err))
	}
	return value, nil
}

func homeDir(cmd *cobra.Command) (string, error) {
	home, err := flagString(cmd, "home")
	if err != nil {
		return "", err
	}
	if home != "" {
		return home, nil
	}
	home, err = os.UserHomeDir()
	if err != nil {
		return "", failure(fmt.Errorf("home directory is unset; pass --home: %w", err))
	}
	return home, nil
}

func locations(cmd *cobra.Command) (home, applications string, err error) {
	if home, err = homeDir(cmd); err != nil {
		return "", "", err
	}
	applications, err = flagString(cmd, "applications")
	return home, applications, err
}

func writeJSON(cmd *cobra.Command, v any) error {
	return failure(json.NewEncoder(cmd.OutOrStdout()).Encode(v))
}

func writeLines(cmd *cobra.Command, lines []string) error {
	out := cmd.OutOrStdout()
	for _, line := range lines {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return failure(err)
		}
	}
	return nil
}

func markRequired(cmd *cobra.Command, name string) {
	if err := cmd.MarkFlagRequired(name); err != nil {
		panic(err)
	}
}
