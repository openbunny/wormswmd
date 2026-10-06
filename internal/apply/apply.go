package apply

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/openbunny/wormswmd/internal/agl"
	"github.com/openbunny/wormswmd/internal/backup"
	"github.com/openbunny/wormswmd/internal/bundle"
	"github.com/openbunny/wormswmd/internal/check"
	"github.com/openbunny/wormswmd/internal/game"
	"github.com/openbunny/wormswmd/internal/plistfix"
	"github.com/openbunny/wormswmd/internal/qt"
	"github.com/openbunny/wormswmd/internal/run"
)

const (
	privateDir     = 0o700
	restoreTimeout = 10 * time.Minute
)

type Options struct {
	App            string
	Home           string
	Applications   string
	QtArchive      string
	QtPrefix       string
	QtSHA256       string
	Force          bool
	BackupDir      string
	InstallRosetta bool
	Preview        bool
	MacOS          string
	Arch           string
	CacheDir       string
	EnvQt          string
	Now            time.Time
	Exec           run.Exec
	EnsureQt       func(context.Context, string) error
}

type Result struct {
	App      string   `json:"app"`
	Backup   string   `json:"backup,omitempty"`
	Changes  []string `json:"changes"`
	Warnings []string `json:"warnings"`
	Already  bool     `json:"already"`
	Preview  bool     `json:"preview"`
}

func Run(ctx context.Context, opt Options) (result Result, err error) {
	if err = ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("apply: %w", err)
	}
	app, err := game.Resolve(ctx, opt.App, opt.Home, opt.Applications)
	if err != nil {
		return Result{}, fmt.Errorf("apply: %w", err)
	}
	if err = refuseEscape(app); err != nil {
		return Result{}, err
	}
	slog.Info("resolved the app", "app", app)
	already, err := alreadyApplied(ctx, app, opt)
	if err != nil {
		return Result{}, err
	}
	if already {
		slog.Info("the app already matches a ready check", "app", app)
		return Result{App: app, Already: true, Preview: opt.Preview, Changes: []string{}, Warnings: []string{}}, nil
	}
	if err = checkHost(ctx, opt); err != nil {
		return Result{}, err
	}
	slog.Info("the Mac passed the host check", "app", app)
	warnings := []string{}
	free, err := freeBytes(app)
	if err != nil {
		return Result{}, err
	}
	if free < minFreeMiB*mib {
		warnings = append(warnings, fmt.Sprintf("%d MiB free; below %d MiB", free/mib, minFreeMiB))
		slog.Warn("free space is below the minimum", "free_mib", free/mib, "minimum_mib", minFreeMiB)
	}
	prefix := opt.QtPrefix
	archive := ""
	if prefix != "" {
		if err = qt.Validate(ctx, prefix); err != nil {
			return Result{}, fmt.Errorf("apply: %w", err)
		}
		slog.Info("using the Qt prefix", "prefix", prefix)
	} else {
		if opt.QtArchive == "" && opt.EnvQt == "" && opt.EnsureQt != nil {
			if err = opt.EnsureQt(ctx, archivePath(opt)); err != nil {
				return Result{}, err
			}
		}
		archive, err = locateArchive(opt)
		if err != nil {
			return Result{}, err
		}
		slog.Info("using the Qt archive", "archive", archive)
	}
	tmp, err := os.MkdirTemp("", "wormswmd-apply-")
	if err != nil {
		return Result{}, fmt.Errorf("apply: %w", err)
	}
	installed := false
	defer func() {
		rmErr := os.RemoveAll(tmp)
		if rmErr == nil {
			return
		}
		if installed {
			err = errors.Join(err, fmt.Errorf("apply: the fix is installed; %s remains: %w", tmp, rmErr))
			return
		}
		err = errors.Join(err, fmt.Errorf("apply: remove %s: %w", tmp, rmErr))
	}()
	if err = os.Chmod(tmp, privateDir); err != nil {
		return Result{}, fmt.Errorf("apply: %w", err)
	}
	if archive != "" {
		prefix = tmpQt(tmp)
		slog.Info("staging the Qt archive", "archive", archive)
		if err = stageArchive(ctx, archive, prefix, opt.QtSHA256); err != nil {
			return Result{}, err
		}
	}
	ex := run.Or(opt.Exec)
	aglBin := ""
	if !opt.Preview {
		slog.Info("building the AGL stub")
		aglBin, err = agl.Build(ctx, tmpAGL(tmp), ex)
		if err != nil {
			return Result{}, fmt.Errorf("apply: %w", err)
		}
	}
	changes, err := planChanges(ctx, app, prefix)
	if err != nil {
		return Result{}, err
	}
	if opt.Preview {
		slog.Info("preview finished", "app", app)
		return Result{App: app, Changes: changes, Warnings: warnings, Preview: true}, nil
	}
	slog.Info("writing the app backup", "app", app)
	backupDir, err := backup.Create(ctx, app, opt.Home, opt.BackupDir, opt.Now)
	if err != nil {
		return Result{}, fmt.Errorf("apply: %w", err)
	}
	result = Result{App: app, Backup: backupDir, Changes: changes, Warnings: warnings}
	slog.Info("app backup written", "backup", backupDir)
	slog.Info("writing the macOS fix", "app", app)
	extra, mutErr := mutate(ctx, app, prefix, aglBin, ex)
	if mutErr != nil {
		slog.Error("apply: mutate failed", "backup", backupDir, "err", mutErr)
		restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreTimeout)
		defer cancel()
		rerr := backup.Restore(restoreCtx, backupDir, app, false)
		if rerr != nil {
			slog.Error("apply: restore failed", "backup", backupDir, "err", rerr)
			rerr = fmt.Errorf("apply: %w", rerr)
		} else {
			slog.Info("restored the app from the backup", "backup", backupDir)
		}
		return result, errors.Join(mutErr, rerr)
	}
	installed = true
	result.Warnings = append(result.Warnings, extra...)
	slog.Info("clearing the quarantine attribute", "app", app)
	if err = clearQuarantine(ctx, app, ex); err != nil {
		return result, err
	}
	slog.Info("deleting Qt window defaults")
	if err = resetWindow(ctx, ex); err != nil {
		return result, err
	}
	slog.Info("apply finished", "app", app, "backup", backupDir)
	return result, nil
}

func alreadyApplied(ctx context.Context, app string, opt Options) (bool, error) {
	if opt.Force {
		return false, nil
	}
	aglDir := appFramework(app, "AGL")
	if _, err := bundle.Binary(aglDir, "AGL"); err != nil {
		if errors.Is(err, bundle.ErrNoBinary) {
			return false, nil
		}
		return false, fmt.Errorf("apply: %w", err)
	}
	version, err := qtCoreShort(app)
	if err != nil {
		if errors.Is(err, bundle.ErrNoBinary) || errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("apply: %w", err)
	}
	if !qt.SameVersion(version) {
		return false, nil
	}
	gaps, err := check.BundleGaps(ctx, app, opt.Exec)
	if err != nil {
		return false, fmt.Errorf("apply: %w", err)
	}
	if len(gaps) > 0 {
		return false, nil
	}
	signed, err := check.Verified(ctx, app, opt.Exec)
	if err != nil {
		return false, fmt.Errorf("apply: %w", err)
	}
	if !signed {
		return false, nil
	}
	keys, err := check.WindowDefaults(ctx, check.Probes{Exec: opt.Exec})
	if err != nil {
		return false, fmt.Errorf("apply: %w", err)
	}
	return !keys, nil
}

func qtCoreShort(app string) (string, error) {
	core := appFramework(app, "QtCore")
	bin, err := bundle.Binary(core, "QtCore")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(plistBeside(bin))
	if err != nil {
		return "", err
	}
	return plistfix.ShortVersion(data)
}

func checkHost(ctx context.Context, opt Options) error {
	version := opt.MacOS
	if version == "" {
		out, err := run.Or(opt.Exec)(ctx, "sw_vers", "-productVersion")
		if err != nil {
			return fmt.Errorf("apply: %w", err)
		}
		version = strings.TrimSpace(string(out))
	}
	major, err := check.Major(version)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if major < check.MinMajor && !opt.Force {
		return fmt.Errorf("apply: macOS %s is below major version %d; pass --force", version, check.MinMajor)
	}
	arch := opt.Arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	if arch != "arm64" {
		return nil
	}
	ex := run.Or(opt.Exec)
	if _, err := ex(ctx, "arch", "-x86_64", "/usr/bin/true"); err != nil {
		if !opt.InstallRosetta {
			return fmt.Errorf("apply: x86_64 cannot run; pass --install-rosetta: %w", err)
		}
		if _, installErr := ex(ctx, "softwareupdate", "--install-rosetta", "--agree-to-license"); installErr != nil {
			return fmt.Errorf("apply: %w", installErr)
		}
	}
	return nil
}

func clearQuarantine(ctx context.Context, app string, ex run.Exec) error {
	if _, err := ex(ctx, "xattr", "-rd", "com.apple.quarantine", app); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	out, err := ex(ctx, "xattr", "-p", "com.apple.quarantine", app)
	if err == nil {
		return fmt.Errorf("apply: quarantine attribute remains on %s", app)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("apply: %w", ctxErr)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("apply: %w", err)
	}
	if bytes.Contains(out, []byte("No such xattr")) {
		return nil
	}
	return fmt.Errorf("apply: %w", err)
}

func resetWindow(ctx context.Context, ex run.Exec) error {
	for _, key := range check.WindowKeys() {
		out, err := ex(ctx, "defaults", "delete", check.WindowDomain, key)
		if err == nil {
			continue
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("apply: %w", ctxErr)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("apply: %w", err)
		}
		if check.DefaultsMissing(out) {
			continue
		}
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}
