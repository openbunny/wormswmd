package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/openbunny/wormswmd/internal/apply"
)

func newApply(preview bool) *cobra.Command {
	use := "apply"
	short := "Write the macOS 26 changes to Worms W.M.D."
	if preview {
		use = "preview"
		short = "Print the macOS 26 changes and do not write them."
	}
	return newApplyCommand(use, short, func(cmd *cobra.Command, opt apply.Options) error {
		return runApply(cmd, opt, preview)
	})
}

func newApplyCommand(use, short string, run func(*cobra.Command, apply.Options) error) *cobra.Command {
	var opt apply.Options
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd, opt)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.App, "app", "", "Path to Worms W.M.D.app.")
	flags.StringVar(&opt.QtArchive, "qt", "", "Path to a Qt 5.15 archive.")
	flags.StringVar(&opt.QtPrefix, "qt-prefix", "", "Path to an extracted Qt 5.15 prefix.")
	flags.StringVar(&opt.QtSHA256, "qt-sha256", "", "SHA-256 pin for the Qt archive, as hex.")
	flags.BoolVar(&opt.Force, "force", false, "Apply when the changes are already present or the macOS major version is below the supported minimum.")
	flags.StringVar(&opt.BackupDir, "backup-dir", "", "Directory that receives the app backup.")
	flags.BoolVar(&opt.InstallRosetta, "install-rosetta", false, "Install Rosetta when Rosetta is absent.")
	return cmd
}

func prepareApply(cmd *cobra.Command, opt apply.Options, preview bool) (apply.Options, error) {
	home, applications, err := locations(cmd)
	if err != nil {
		return apply.Options{}, err
	}
	opt.Home = home
	opt.Applications = applications
	opt.Preview = preview
	opt.EnvQt = os.Getenv("WORMSWMD_QT")
	cache, cacheErr := os.UserCacheDir()
	chosen, err := applyCache(opt.QtArchive, opt.EnvQt, opt.QtPrefix, cache, cacheErr)
	if err != nil {
		return apply.Options{}, failure(err)
	}
	opt.CacheDir = chosen
	opt.Now = time.Now()
	return opt, nil
}

func runApply(cmd *cobra.Command, opt apply.Options, preview bool) error {
	opt, err := prepareApply(cmd, opt, preview)
	if err != nil {
		return err
	}
	result, err := apply.Run(cmd.Context(), opt)
	if err != nil {
		return failure(err)
	}
	return writeApply(cmd, result)
}

func applyCache(qt, envQt, prefix, cache string, cacheErr error) (string, error) {
	if cacheErr == nil {
		return cache, nil
	}
	if qt != "" || envQt != "" || prefix != "" {
		return "", nil
	}
	return "", fmt.Errorf("user cache directory: %w; pass --qt or --qt-prefix, or set WORMSWMD_QT", cacheErr)
}

func writeApply(cmd *cobra.Command, result apply.Result) error {
	return emit(cmd, result, func() error {
		headline := "Applied: the macOS 26 changes are written to Worms W.M.D."
		switch {
		case result.Already:
			headline = "Already fixed: Worms W.M.D needs no changes."
		case result.Preview:
			headline = "Preview: apply would make these changes to Worms W.M.D."
		}
		return writeLines(cmd, append([]string{headline}, applyDetails(result)...))
	})
}

func applyDetails(result apply.Result) []string {
	lines := []string{"  App: " + result.App}
	if result.Backup != "" {
		lines = append(lines, "  Backup: "+result.Backup)
	}
	lines = append(lines, bullets("Changes", result.Changes)...)
	return append(lines, bullets("Warnings", result.Warnings)...)
}

func bullets(label string, items []string) []string {
	if len(items) == 0 {
		return nil
	}
	lines := []string{"  " + label + ":"}
	for _, item := range items {
		lines = append(lines, "    - "+item)
	}
	return lines
}
