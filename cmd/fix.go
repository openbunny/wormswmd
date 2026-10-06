package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/openbunny/wormswmd/internal/apply"
	"github.com/openbunny/wormswmd/internal/check"
	"github.com/openbunny/wormswmd/internal/qt"
)

func newFix() *cobra.Command {
	return newApplyCommand("fix", "Download the pinned Qt archive when needed, write the macOS 26 changes, and check the app.", runFix)
}

func runFix(cmd *cobra.Command, opt apply.Options) error {
	opt, err := prepareApply(cmd, opt, false)
	if err != nil {
		return err
	}
	if opt.QtPrefix == "" && opt.QtArchive == "" && opt.EnvQt == "" {
		opt.EnsureQt = ensurePinnedQt
	}
	result, err := apply.Run(cmd.Context(), opt)
	if err != nil {
		return failure(err)
	}
	report, err := check.Evaluate(cmd.Context(), result.App, opt.Home, opt.Applications, check.Probes{})
	if err != nil {
		return failure(err)
	}
	if err := writeFix(cmd, result, report); err != nil {
		return err
	}
	slog.Debug("check after the fix returned", "app", report.App, "ready", report.Ready, "exit", report.Exit)
	if report.Exit == 0 {
		return nil
	}
	return &ExitError{Code: report.Exit}
}

var fetchQt = qt.Fetch

func ensurePinnedQt(ctx context.Context, path string) error {
	err := qt.VerifyFile(ctx, path, qt.PinSHA256)
	if err == nil {
		slog.Debug("pinned Qt archive is present", "path", path)
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("fix: %w", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("The downloaded Qt archive failed verification; downloading it again", "path", path, "err", err)
	}
	slog.Info("Downloading the Qt archive to " + path)
	if err := fetchQt(ctx, path, qt.ArchiveURL, qt.PinSHA256); err != nil {
		return fmt.Errorf("fix: %w", err)
	}
	slog.Debug("pinned Qt archive is ready", "path", path)
	return nil
}

func writeFix(cmd *cobra.Command, result apply.Result, report check.Report) error {
	return emit(cmd, fixJSON{Apply: result, Check: report}, func() error {
		headline := fmt.Sprintf("Fixed, but not ready: Worms W.M.D will not open (exit %d).", report.Exit)
		switch {
		case report.Ready && result.Already:
			headline = "Already fixed: Worms W.M.D is ready to open."
		case report.Ready:
			headline = "Fixed: Worms W.M.D is ready to open."
		}
		lines := append([]string{headline}, applyDetails(result)...)
		return writeLines(cmd, append(lines, checkDetails(report)...))
	})
}

type fixJSON struct {
	Apply apply.Result `json:"apply"`
	Check check.Report `json:"check"`
}
