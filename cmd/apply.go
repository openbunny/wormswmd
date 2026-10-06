package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/openbunny/wormswmd/internal/apply"
	"github.com/openbunny/wormswmd/internal/check"
)

func newApply(preview bool) *cobra.Command {
	use := "apply"
	short := "Write the macOS 26 changes to Worms W.M.D."
	if preview {
		use = "preview"
		short = "Print the macOS 26 changes and do not write them."
	}
	return newApplyCommand(use, short, preview, func(cmd *cobra.Command, opt apply.Options) error {
		return runApply(cmd, opt, preview)
	})
}

func newApplyCommand(use, short string, preview bool, run func(*cobra.Command, apply.Options) error) *cobra.Command {
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
	flags.StringVar(&opt.QtArchive, "qt", "", "Path to a Qt 5.15 archive. The archive is verified against the pinned SHA-256 or --qt-sha256.")
	flags.StringVar(&opt.QtPrefix, "qt-prefix", "", "Path to an extracted Qt 5.15 prefix. The files are not verified against the pinned SHA-256.")
	flags.StringVar(&opt.QtSHA256, "qt-sha256", "", "SHA-256 as hex that replaces the pinned SHA-256 for the archive given by --qt or WORMSWMD_QT.")
	if preview {
		flags.BoolVar(&opt.Force, "force", false, fmt.Sprintf("Plan the changes when they are already present or the macOS major version is below %d. Nothing is written.", check.MinMajor))
		return cmd
	}
	flags.BoolVar(&opt.Force, "force", false, fmt.Sprintf("Write the changes when they are already present or the macOS major version is below %d.", check.MinMajor))
	flags.StringVar(&opt.BackupDir, "backup-dir", "", "Directory that receives the app backup. The default is under the home directory.")
	flags.BoolVar(&opt.InstallRosetta, "install-rosetta", false, "Install Rosetta when it is absent.")
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
	if opt.QtSHA256 != "" && opt.QtArchive == "" && opt.EnvQt == "" {
		return apply.Options{}, failure(errors.New("--qt-sha256 applies to an archive given by --qt or WORMSWMD_QT; pass one of them or remove --qt-sha256"))
	}
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
