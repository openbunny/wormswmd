package apply

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/openbunny/wormswmd/internal/agl"
	"github.com/openbunny/wormswmd/internal/configurl"
	"github.com/openbunny/wormswmd/internal/macho"
	"github.com/openbunny/wormswmd/internal/plistfix"
	"github.com/openbunny/wormswmd/internal/qt"
	"github.com/openbunny/wormswmd/internal/run"
	"github.com/openbunny/wormswmd/internal/safe"
	"github.com/openbunny/wormswmd/internal/tree"
)

func locateArchive(opt Options) (string, error) {
	path := archivePath(opt)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if opt.QtArchive != "" || opt.EnvQt != "" {
				return "", fmt.Errorf("apply: Qt archive %s is absent", path)
			}
			return "", fmt.Errorf("apply: Qt archive %s is absent; run wormswmd qt fetch or pass --qt", path)
		}
		return "", fmt.Errorf("apply: stat %s: %w", path, err)
	}
	return path, nil
}

func archivePath(opt Options) string {
	switch {
	case opt.QtArchive != "":
		return opt.QtArchive
	case opt.EnvQt != "":
		return opt.EnvQt
	case opt.CacheDir != "":
		return filepath.Join(opt.CacheDir, "wormswmd", qt.ArchiveName)
	default:
		return filepath.Join(opt.Home, ".cache", "wormswmd-fix", qt.ArchiveName)
	}
}

func stageArchive(ctx context.Context, archive, dest, pin string) error {
	sum := qt.PinSHA256
	if pin != "" {
		var err error
		sum, err = qt.NormalizePin(pin)
		if err != nil {
			return fmt.Errorf("apply: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if err := qt.VerifyFile(ctx, archive, sum); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if err := qt.ExtractFile(ctx, archive, dest); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if err := qt.Validate(ctx, dest); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}

func planChanges(ctx context.Context, app, prefix string) ([]string, error) {
	changes := []string{
		"Build an AGL stub.",
		"Replace Qt frameworks from " + prefix,
	}
	for _, root := range configRoots(app) {
		for _, name := range root.names {
			data, ok, err := readConfig(filepath.Join(root.dir, name))
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			if _, n := configurl.Rewrite(string(data)); n > 0 {
				changes = append(changes, "Rewrite "+filepath.Base(name))
			}
		}
	}
	_, _, keys, err := loadFixedPlist(ctx, app)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		changes = append(changes, "Set Info.plist "+key)
	}
	changes = append(changes, "Rewrite Mach-O install names.", "Ad-hoc sign the app.")
	return changes, nil
}

func mutate(ctx context.Context, app, prefix, aglBin string, ex run.Exec) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("apply: %w", err)
	}
	if err := refuseEscape(app); err != nil {
		return nil, err
	}
	slog.Info("replacing Qt frameworks", "app", app)
	if err := copyFrameworks(ctx, app, prefix); err != nil {
		return nil, err
	}
	slog.Info("copying Qt dependency libraries", "app", app)
	if err := copyDylibs(ctx, app, prefix); err != nil {
		return nil, err
	}
	slog.Info("removing leftover Qt plug-ins", "app", app)
	if err := removePlugInDirs(app); err != nil {
		return nil, err
	}
	slog.Info("copying libqcocoa.dylib", "app", app)
	if err := copyPlugIn(ctx, app, prefix, filepath.Join("platforms", "libqcocoa.dylib")); err != nil {
		return nil, err
	}
	slog.Info("copying Qt image format plug-ins", "app", app)
	if err := copyImageFormats(ctx, app, prefix); err != nil {
		return nil, err
	}
	slog.Info("installing the AGL stub", "app", app)
	if err := installAGL(ctx, app, aglBin); err != nil {
		return nil, err
	}
	slog.Info("rewriting config URLs", "app", app)
	if err := rewriteConfigs(app); err != nil {
		return nil, err
	}
	slog.Info("writing Info.plist", "app", app)
	if err := writePlist(ctx, app); err != nil {
		return nil, err
	}
	slog.Info("rewriting Mach-O install names", "app", app)
	warnings, err := macho.Rewrite(ctx, app, ex)
	if err != nil {
		return nil, fmt.Errorf("apply: %w", err)
	}
	slog.Info("signing the app", "app", app)
	if _, err := ex(ctx, "codesign", "--force", "--deep", "--sign", "-", app); err != nil {
		return nil, fmt.Errorf("apply: %w", err)
	}
	if _, err := ex(ctx, "codesign", "--verify", "--deep", "--strict", app); err != nil {
		return nil, fmt.Errorf("apply: %w", err)
	}
	return warnings, nil
}

func copyFrameworks(ctx context.Context, app, prefix string) error {
	names, err := qt.Frameworks(ctx, appFrameworks(app), prefix)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	for _, name := range names {
		dest := appFramework(app, name)
		if err := insideApp(app, dest); err != nil {
			return err
		}
		src := filepath.Join(prefix, "Frameworks", name+".framework")
		if err := replacePath(ctx, src, dest); err != nil {
			return err
		}
	}
	return nil
}

func copyDylibs(ctx context.Context, app, prefix string) error {
	names, err := qt.DependencyDylibs(ctx, prefix)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	for _, name := range names {
		dest := filepath.Join(appFrameworks(app), name)
		if err := insideApp(app, dest); err != nil {
			return err
		}
		if err := replacePath(ctx, filepath.Join(prefix, "Frameworks", name), dest); err != nil {
			return err
		}
	}
	return nil
}

func replacePath(ctx context.Context, src, dest string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	parent := filepath.Dir(dest)
	base := filepath.Base(dest)
	staged := filepath.Join(parent, ".wormswmd-new-"+base)
	prior := filepath.Join(parent, ".wormswmd-old-"+base)
	if err := os.RemoveAll(staged); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if err := tree.Copy(ctx, src, staged); err != nil {
		if rmErr := os.RemoveAll(staged); rmErr != nil {
			return fmt.Errorf("apply: copy %s: %w; remove %s: %w", src, err, staged, rmErr)
		}
		return fmt.Errorf("apply: %w", err)
	}
	had := false
	if _, err := os.Lstat(dest); err == nil {
		had = true
		if err := os.RemoveAll(prior); err != nil {
			return joinRemove(staged, fmt.Errorf("apply: %w", err))
		}
		if err := os.Rename(dest, prior); err != nil {
			return joinRemove(staged, fmt.Errorf("apply: %w", err))
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return joinRemove(staged, fmt.Errorf("apply: %w", err))
	}
	if err := os.Rename(staged, dest); err != nil {
		if had {
			if backErr := os.Rename(prior, dest); backErr != nil {
				return fmt.Errorf("apply: rename %s: %w; restore %s: %w", staged, err, dest, backErr)
			}
		}
		return joinRemove(staged, fmt.Errorf("apply: %w", err))
	}
	if !had {
		return nil
	}
	if err := os.RemoveAll(prior); err != nil {
		return fmt.Errorf("apply: replacement is at %s; previous tree remains at %s: %w", dest, prior, err)
	}
	return nil
}

func joinRemove(path string, err error) error {
	if rmErr := os.RemoveAll(path); rmErr != nil {
		return fmt.Errorf("%w; remove %s: %w", err, path, rmErr)
	}
	return err
}

func removePlugInDirs(app string) error {
	for _, name := range []string{"accessible", "printsupport"} {
		dest := filepath.Join(app, "Contents", "PlugIns", name)
		if err := insideApp(app, dest); err != nil {
			return err
		}
		if _, err := os.Lstat(dest); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return fmt.Errorf("apply: %w", err)
		}
		if err := os.RemoveAll(dest); err != nil {
			return fmt.Errorf("apply: %w", err)
		}
	}
	return nil
}

func copyPlugIn(ctx context.Context, app, prefix, rel string) error {
	dest := filepath.Join(app, "Contents", "PlugIns", rel)
	if err := insideApp(app, dest); err != nil {
		return err
	}
	if err := tree.Copy(ctx, filepath.Join(prefix, "PlugIns", rel), dest); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}

func copyImageFormats(ctx context.Context, app, prefix string) error {
	haveSharp, err := sharpyuvPresent(prefix)
	if err != nil {
		return err
	}
	dir := filepath.Join(prefix, "PlugIns", "imageformats")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".dylib") {
			continue
		}
		if !haveSharp && strings.HasPrefix(name, "libqwebp") {
			continue
		}
		if err := copyPlugIn(ctx, app, prefix, filepath.Join("imageformats", name)); err != nil {
			return err
		}
	}
	return nil
}

func sharpyuvPresent(prefix string) (bool, error) {
	_, err := os.Lstat(filepath.Join(prefix, "Frameworks", "libsharpyuv.0.dylib"))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("apply: %w", err)
}

func installAGL(ctx context.Context, app, aglBin string) error {
	dest := appFramework(app, "AGL")
	if err := insideApp(app, dest); err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if err := agl.Install(ctx, dest, aglBin); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}

func rewriteConfigs(app string) error {
	for _, root := range configRoots(app) {
		for _, name := range root.names {
			path := filepath.Join(root.dir, name)
			data, ok, err := readConfig(path)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			next, n := configurl.Rewrite(string(data))
			if n == 0 {
				continue
			}
			if err := insideApp(app, path); err != nil {
				return err
			}
			info, err := os.Lstat(path)
			if err != nil {
				return fmt.Errorf("apply: %w", err)
			}
			if err := os.WriteFile(path, []byte(next), info.Mode().Perm()); err != nil {
				return fmt.Errorf("apply: %w", err)
			}
		}
	}
	return nil
}

func writePlist(ctx context.Context, app string) error {
	original, fixed, keys, err := loadFixedPlist(ctx, app)
	if err != nil {
		return err
	}
	if bytes.Equal(original, fixed) && len(keys) == 0 {
		return nil
	}
	path := infoPlist(app)
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if err := os.WriteFile(path, fixed, info.Mode().Perm()); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}

func loadFixedPlist(ctx context.Context, app string) ([]byte, []byte, []string, error) {
	data, err := os.ReadFile(infoPlist(app))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("apply: %w", err)
	}
	xmlData, err := plistfix.AsXML(ctx, data)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("apply: %w", err)
	}
	fixed, keys, err := plistfix.Fix(xmlData)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("apply: %w", err)
	}
	return data, fixed, keys, nil
}

type configRoot struct {
	dir   string
	names []string
}

func configRoots(app string) []configRoot {
	return []configRoot{
		{filepath.Join(app, "Contents", "Resources", configurl.DataOSXName), configurl.DataOSX()},
		{filepath.Join(app, "Contents", "Resources", configurl.CommonDataName), configurl.CommonData()},
	}
}

func readConfig(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("apply: %w", err)
	}
	return data, true, nil
}

func refuseEscape(app string) error {
	for _, path := range []string{
		appFramework(app, "QtCore"),
		filepath.Join(app, "Contents", "PlugIns", "platforms"),
		filepath.Join(app, "Contents", "MacOS", "Worms W.M.D"),
		infoPlist(app),
		filepath.Join(app, "Contents", "Resources", "DataOSX", "SteamConfig.txt"),
		filepath.Join(app, "Contents", "Resources", "CommonData", "AnalyticsConfig.txt"),
		filepath.Join(app, "Contents", "_CodeSignature", "CodeResources"),
	} {
		if err := insideApp(app, path); err != nil {
			return err
		}
	}
	return nil
}

func insideApp(app, path string) error {
	if err := safe.IntermediateEscape(app, path); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}

func appFrameworks(app string) string {
	return filepath.Join(app, "Contents", "Frameworks")
}

func appFramework(app, name string) string {
	return filepath.Join(appFrameworks(app), name+".framework")
}

func infoPlist(app string) string {
	return filepath.Join(app, "Contents", "Info.plist")
}

func plistBeside(bin string) string {
	return filepath.Join(filepath.Dir(bin), "Resources", "Info.plist")
}

func tmpQt(tmp string) string {
	return filepath.Join(tmp, "qt")
}

func tmpAGL(tmp string) string {
	return filepath.Join(tmp, "agl")
}
