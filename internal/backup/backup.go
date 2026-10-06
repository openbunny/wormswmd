package backup

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/openbunny/wormswmd/internal/game"
	"github.com/openbunny/wormswmd/internal/progress"
	"github.com/openbunny/wormswmd/internal/safe"
	"github.com/openbunny/wormswmd/internal/tree"
)

const (
	manifestName = "BACKUP_MANIFEST.tsv"
	metadataName = "BACKUP_METADATA.tsv"

	backupDirMode  = 0o700
	backupFileMode = 0o600
	appDirMode     = 0o755
	maxNlink       = 1

	stampLayout = "WormsWMD-Backup-20060102-150405"

	dirDocuments     = "Documents"
	dirContents      = "Contents"
	dirResources     = "Resources"
	dirFrameworks    = "Frameworks"
	dirPlugIns       = "PlugIns"
	dirMacOS         = "MacOS"
	dirDataOSX       = "DataOSX"
	dirCommonData    = "CommonData"
	dirCodeSignature = "_CodeSignature"
	fileInfoPlist    = "Info.plist"
)

type fileEntry struct {
	path    string
	symlink bool
	digest  string
	size    int64
}

type copyProgress struct {
	tree.Tally
	counter *progress.Counter
}

func newCopyProgress(label string, total int) *copyProgress {
	return &copyProgress{counter: progress.Count(label, total)}
}

func (c *copyProgress) onFile(size int64) {
	c.Add(size)
	c.counter.Add(1)
}

type restorePlan struct {
	dataOSX bool
	common  bool
	sig     optionalBool
}

func Create(ctx context.Context, app, home, explicit string, now time.Time) (dir string, err error) {
	if err = ctx.Err(); err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	if err = game.Valid(ctx, app); err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	source, err := game.Source(ctx, app)
	if err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	if err = knownSource(source); err != nil {
		return "", err
	}
	if err = rejectMetaValue(keyAppPath, app); err != nil {
		return "", err
	}
	if err = rejectEscapes(app); err != nil {
		return "", err
	}
	digest, size, err := hashFile(ctx, game.Executable(app))
	if err != nil {
		return "", err
	}
	contents := filepath.Join(app, dirContents)
	for _, name := range []string{dirFrameworks, dirPlugIns, dirMacOS} {
		if err := requireKind(filepath.Join(contents, name), true); err != nil {
			return "", err
		}
	}
	if err := requireKind(filepath.Join(contents, fileInfoPlist), false); err != nil {
		return "", err
	}
	for _, path := range []string{
		filepath.Join(contents, dirResources, dirDataOSX),
		filepath.Join(contents, dirResources, dirCommonData),
		filepath.Join(contents, dirCodeSignature),
	} {
		if _, err := isDir(path); err != nil {
			return "", err
		}
	}
	dest := backupPath(home, explicit, now)
	if err = rejectInsideApp(app, dest); err != nil {
		return "", err
	}
	done := progress.Begin("Backing up Worms W.M.D to " + dest)
	if err = makeBackupDir(dest); err != nil {
		return "", err
	}
	defer func() {
		if err == nil {
			return
		}
		err = errors.Join(err, removeCreated(dest))
	}()
	optionals := []struct {
		src       string
		dst       string
		signature bool
	}{
		{filepath.Join(contents, dirResources, dirDataOSX), dirDataOSX, false},
		{filepath.Join(contents, dirResources, dirCommonData), dirCommonData, false},
		{filepath.Join(contents, dirCodeSignature), dirCodeSignature, true},
	}
	required := []string{dirFrameworks, dirPlugIns, dirMacOS, fileInfoPlist}
	var sources []string
	for _, name := range required {
		sources = append(sources, filepath.Join(contents, name))
	}
	for _, opt := range optionals {
		present, err := isDir(opt.src)
		if err != nil {
			return "", err
		}
		if present {
			sources = append(sources, opt.src)
		}
	}
	total, err := tree.Measure(ctx, sources...)
	if err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	slog.Debug("measured the app", "files", total.Files, "bytes", total.Bytes, "sources", len(sources))
	copied := newCopyProgress("Copying the app files", total.Files)
	for _, name := range required {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("backup: %w", err)
		}
		slog.Debug("copying", "from", filepath.Join(contents, name), "to", filepath.Join(dest, name))
		if err := tree.CopyCounted(ctx, filepath.Join(contents, name), filepath.Join(dest, name), copied.onFile); err != nil {
			return "", fmt.Errorf("backup: %w", err)
		}
	}
	sig := false
	for _, opt := range optionals {
		copiedDir, err := copyOptional(ctx, opt.src, filepath.Join(dest, opt.dst), copied.onFile)
		if err != nil {
			return "", err
		}
		sig = sig || (opt.signature && copiedDir)
	}
	meta := metadata{
		appPath: app,
		source:  source,
		exeHash: digest,
		exeSize: size,
		sig:     optionalBool{present: true, value: sig},
	}
	if err := writeMetadata(dest, meta); err != nil {
		return "", err
	}
	if err := writeManifest(ctx, dest); err != nil {
		return "", err
	}
	if err := Verify(ctx, dest); err != nil {
		return "", err
	}
	done("Backed up Worms W.M.D: " + copied.String())
	return dest, nil
}

func Verify(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	done := progress.Begin("Verifying the backup")
	meta, err := loadMetadata(dir)
	if err != nil {
		return err
	}
	data, err := readRegular(filepath.Join(dir, manifestName))
	if err != nil {
		return err
	}
	rows, err := parseManifest(data)
	if err != nil {
		return err
	}
	entries, err := collectFiles(ctx, dir)
	if err != nil {
		return err
	}
	got := make(map[string]fileEntry, len(entries))
	for _, entry := range entries {
		if _, ok := got[entry.path]; ok {
			return fmt.Errorf("backup: %s appears twice in the backup tree; each path must appear once; create a new backup", entry.path)
		}
		got[entry.path] = entry
	}
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if _, ok := seen[row.path]; ok {
			return fmt.Errorf("backup: duplicate manifest path %s", row.path)
		}
		seen[row.path] = struct{}{}
		entry, ok := got[row.path]
		if !ok {
			return missingListed(dir, row.path)
		}
		if err := matchEntry(row, entry); err != nil {
			return err
		}
	}
	var unlisted []string
	for path := range got {
		if _, ok := seen[path]; !ok {
			unlisted = append(unlisted, path)
		}
	}
	if len(unlisted) > 0 {
		return fmt.Errorf("backup: unlisted path %s; every file in the backup must be listed in its manifest; remove the file or create a new backup", slices.Min(unlisted))
	}
	digest, size, err := hashFile(ctx, executablePath(dir))
	if err != nil {
		return err
	}
	exe := executableRel()
	if digest != meta.exeHash {
		return fmt.Errorf("backup: %s digest is %s, metadata lists %s", exe, digest, meta.exeHash)
	}
	if size != meta.exeSize {
		return fmt.Errorf("backup: %s size is %d bytes, metadata lists %d bytes", exe, size, meta.exeSize)
	}
	done("Verified the backup")
	return nil
}

func Restore(ctx context.Context, dir, app string, force bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	done := progress.Begin("Restoring Worms W.M.D from the backup")
	if err := Verify(ctx, dir); err != nil {
		return "", err
	}
	meta, err := loadMetadata(dir)
	if err != nil {
		return "", err
	}
	if app == "" {
		app = meta.appPath
	}
	if app == "" {
		return "", errors.New("backup: game_app_path is empty; pass --app")
	}
	if meta.appPath != app && !force {
		return "", fmt.Errorf("backup: game_app_path %s differs from %s; pass --force", meta.appPath, app)
	}
	plan, err := buildPlan(dir, meta)
	if err != nil {
		return "", err
	}
	if err := destinationChecks(app, plan); err != nil {
		return "", err
	}
	copied, err := restoreInto(ctx, dir, app, plan)
	if err != nil {
		return "", err
	}
	done("Restored Worms W.M.D: " + copied.String())
	return app, nil
}

func backupPath(home, explicit string, now time.Time) string {
	if explicit != "" {
		return explicit
	}
	when := now
	if when.IsZero() {
		when = time.Now()
	}
	return filepath.Join(home, dirDocuments, when.UTC().Format(stampLayout))
}

func makeBackupDir(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), backupDirMode); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if err := os.Mkdir(path, backupDirMode); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if err := os.Chmod(path, backupDirMode); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	return nil
}

func copyOptional(ctx context.Context, src, dst string, onFile func(int64)) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("backup: %w", err)
	}
	info, err := os.Lstat(src)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("backup: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("backup: %s is a symlink", src)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("backup: %s is not a directory", src)
	}
	slog.Debug("copying", "from", src, "to", dst)
	if err := tree.CopyCounted(ctx, src, dst, onFile); err != nil {
		return false, fmt.Errorf("backup: %w", err)
	}
	return true, nil
}

func writeMetadata(dir string, meta metadata) error {
	rows := []struct{ key, value string }{
		{keyAppPath, meta.appPath},
		{keySource, meta.source},
		{keyExeHash, meta.exeHash},
		{keyExeSize, strconv.FormatInt(meta.exeSize, 10)},
		{keyTreeComplete, boolTrue},
		{keySigPresent, strconv.FormatBool(meta.sig.value)},
	}
	var b strings.Builder
	b.WriteString(metadataHeaderV1)
	b.WriteByte('\n')
	for _, row := range rows {
		if err := rejectMetaValue(row.key, row.value); err != nil {
			return err
		}
		b.WriteString(row.key)
		b.WriteByte('\t')
		b.WriteString(row.value)
		b.WriteByte('\n')
	}
	return writePrivate(filepath.Join(dir, metadataName), b.String())
}

func rejectMetaValue(key, value string) error {
	if strings.ContainsAny(value, "\t\n") {
		return fmt.Errorf("backup: metadata %s contains a tab or a newline", key)
	}
	return nil
}

func writeManifest(ctx context.Context, dir string) error {
	entries, err := collectFiles(ctx, dir)
	if err != nil {
		return err
	}
	slices.SortFunc(entries, func(a, b fileEntry) int {
		return cmp.Compare(a.path, b.path)
	})
	var b strings.Builder
	b.WriteString(manifestHeaderV1)
	b.WriteByte('\n')
	b.WriteString(manifestColumnHeader)
	b.WriteByte('\n')
	for _, entry := range entries {
		digest := entry.digest
		if entry.symlink {
			digest = symlinkPrefix + digest
		}
		fmt.Fprintf(&b, "%s\t%d\t%s\n", digest, entry.size, entry.path)
	}
	return writePrivate(filepath.Join(dir, manifestName), b.String())
}

func collectFiles(ctx context.Context, dir string) ([]fileEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}
	var entries []fileEntry
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("backup: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("backup: %w", err)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("backup: %w", err)
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		clean, err := safe.CleanRel(rel)
		if err != nil {
			return fmt.Errorf("backup: %w", err)
		}
		clean = filepath.ToSlash(clean)
		switch {
		case d.Type()&os.ModeSymlink != 0:
			if clean == manifestName {
				return fmt.Errorf("backup: %s is a symlink", clean)
			}
			target, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("backup: %w", err)
			}
			digest, size := hashText(target)
			entries = append(entries, fileEntry{path: clean, symlink: true, digest: digest, size: size})
		case d.IsDir():
			return nil
		case clean == manifestName:
			return nil
		case d.Type().IsRegular():
			info, err := d.Info()
			if err != nil {
				return fmt.Errorf("backup: %w", err)
			}
			n, err := linkCount(info)
			if err != nil {
				return err
			}
			if n > maxNlink {
				return fmt.Errorf("backup: hard link %s has more than %d link; copy the file so it has one link, then retry", clean, maxNlink)
			}
			digest, size, err := hashFile(ctx, path)
			if err != nil {
				return err
			}
			entries = append(entries, fileEntry{path: clean, digest: digest, size: size})
		default:
			return fmt.Errorf("backup: %s is not a regular file, directory or symlink; remove it from the app, then retry", clean)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func matchEntry(row manifestRow, entry fileEntry) error {
	if row.symlink != entry.symlink {
		if row.symlink {
			return fmt.Errorf("backup: %s is not a symlink", row.path)
		}
		return fmt.Errorf("backup: %s is not a regular file", row.path)
	}
	if row.digest != entry.digest {
		return fmt.Errorf("backup: %s digest is %s, manifest lists %s", row.path, entry.digest, row.digest)
	}
	if row.size != entry.size {
		return fmt.Errorf("backup: %s size is %d bytes, manifest lists %d bytes", row.path, entry.size, row.size)
	}
	return nil
}

func missingListed(dir, path string) error {
	full := filepath.Join(dir, filepath.FromSlash(path))
	info, err := os.Lstat(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("backup: missing path %s; the manifest lists it but the backup does not hold it; the backup is incomplete, create a new one", path)
		}
		return fmt.Errorf("backup: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("backup: path %s is a directory", path)
	}
	return fmt.Errorf("backup: missing path %s; the manifest lists it but the backup does not hold it; the backup is incomplete, create a new one", path)
}

func buildPlan(dir string, meta metadata) (restorePlan, error) {
	for _, name := range []string{dirFrameworks, dirPlugIns, dirMacOS} {
		if err := requireKind(filepath.Join(dir, name), true); err != nil {
			return restorePlan{}, err
		}
	}
	if err := requireKind(filepath.Join(dir, fileInfoPlist), false); err != nil {
		return restorePlan{}, err
	}
	if meta.sig.present && meta.sig.value {
		if err := requireKind(filepath.Join(dir, dirCodeSignature), true); err != nil {
			return restorePlan{}, err
		}
	}
	dataOSX, err := isDir(filepath.Join(dir, dirDataOSX))
	if err != nil {
		return restorePlan{}, err
	}
	common, err := isDir(filepath.Join(dir, dirCommonData))
	if err != nil {
		return restorePlan{}, err
	}
	return restorePlan{dataOSX: dataOSX, common: common, sig: meta.sig}, nil
}

func destinationChecks(app string, plan restorePlan) error {
	dests := []string{
		filepath.Join(app, dirContents, dirFrameworks),
		filepath.Join(app, dirContents, dirPlugIns),
		filepath.Join(app, dirContents, dirMacOS),
		filepath.Join(app, dirContents, fileInfoPlist),
	}
	if plan.dataOSX {
		dests = append(dests, filepath.Join(app, dirContents, dirResources, dirDataOSX))
	}
	if plan.common {
		dests = append(dests, filepath.Join(app, dirContents, dirResources, dirCommonData))
	}
	if plan.sig.present {
		dests = append(dests, filepath.Join(app, dirContents, dirCodeSignature))
	}
	for _, dst := range dests {
		if err := contained(app, dst); err != nil {
			return err
		}
	}
	return nil
}

type staged struct {
	dst    string
	fresh  string
	prior  string
	placed bool
}

var renamePath = os.Rename

func restoreInto(ctx context.Context, dir, app string, plan restorePlan) (tree.Tally, error) {
	contents := filepath.Join(app, dirContents)
	jobs := []struct{ src, dst string }{
		{dirFrameworks, filepath.Join(contents, dirFrameworks)},
		{dirPlugIns, filepath.Join(contents, dirPlugIns)},
		{dirMacOS, filepath.Join(contents, dirMacOS)},
		{fileInfoPlist, filepath.Join(contents, fileInfoPlist)},
	}
	if plan.dataOSX {
		jobs = append(jobs, struct{ src, dst string }{dirDataOSX, filepath.Join(contents, dirResources, dirDataOSX)})
	}
	if plan.common {
		jobs = append(jobs, struct{ src, dst string }{dirCommonData, filepath.Join(contents, dirResources, dirCommonData)})
	}
	if plan.sig.present && plan.sig.value {
		jobs = append(jobs, struct{ src, dst string }{dirCodeSignature, filepath.Join(contents, dirCodeSignature)})
	}
	sources := make([]string, 0, len(jobs))
	for _, job := range jobs {
		sources = append(sources, filepath.Join(dir, job.src))
	}
	total, err := tree.Measure(ctx, sources...)
	if err != nil {
		return tree.Tally{}, fmt.Errorf("backup: %w", err)
	}
	copied := newCopyProgress("Copying the backup files", total.Files)
	var stagedItems []staged
	for i, job := range jobs {
		slog.Debug("staging", "from", sources[i], "to", job.dst)
		item, err := stageCopy(ctx, app, sources[i], job.dst, copied.onFile)
		if err != nil {
			return tree.Tally{}, errors.Join(err, discardFresh(stagedItems))
		}
		stagedItems = append(stagedItems, item)
	}
	if err := placeStaged(ctx, stagedItems); err != nil {
		return tree.Tally{}, err
	}
	if plan.sig.present && !plan.sig.value {
		sig := filepath.Join(contents, dirCodeSignature)
		spare, err := moveAside(ctx, app, sig)
		if err != nil {
			return tree.Tally{}, undoStaged(ctx, stagedItems, err)
		}
		if spare != "" {
			stagedItems = append(stagedItems, staged{dst: sig, prior: spare})
		}
	}
	if err := removePriors(stagedItems); err != nil {
		return tree.Tally{}, err
	}
	return copied.Tally, nil
}

func stageCopy(ctx context.Context, app, src, dst string, onFile func(int64)) (staged, error) {
	if err := ctx.Err(); err != nil {
		return staged{}, fmt.Errorf("backup: %w", err)
	}
	if err := contained(app, dst); err != nil {
		return staged{}, err
	}
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, appDirMode); err != nil {
		return staged{}, fmt.Errorf("backup: %w", err)
	}
	info, err := os.Lstat(src)
	if err != nil {
		return staged{}, fmt.Errorf("backup: %w", err)
	}
	if info.IsDir() {
		dir, err := os.MkdirTemp(parent, ".wormswmd-restore-")
		if err != nil {
			return staged{}, fmt.Errorf("backup: %w", err)
		}
		if err := tree.CopyCounted(ctx, src, dir, onFile); err != nil {
			return staged{}, errors.Join(fmt.Errorf("backup: %w", err), os.RemoveAll(dir))
		}
		if err := os.Chmod(dir, appDirMode); err != nil {
			return staged{}, errors.Join(fmt.Errorf("backup: %w", err), os.RemoveAll(dir))
		}
		return staged{dst: dst, fresh: dir}, nil
	}
	f, err := os.CreateTemp(parent, ".wormswmd-restore-")
	if err != nil {
		return staged{}, fmt.Errorf("backup: %w", err)
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		return staged{}, errors.Join(fmt.Errorf("backup: %w", err), os.Remove(name))
	}
	if err := os.Remove(name); err != nil {
		return staged{}, fmt.Errorf("backup: %w", err)
	}
	if err := tree.CopyCounted(ctx, src, name, onFile); err != nil {
		return staged{}, errors.Join(fmt.Errorf("backup: %w", err), os.RemoveAll(name))
	}
	return staged{dst: dst, fresh: name}, nil
}

func placeStaged(ctx context.Context, items []staged) error {
	for i := range items {
		prior, err := spareName(filepath.Dir(items[i].dst))
		if err != nil {
			return abortStaged(ctx, items, i, err)
		}
		_, err = os.Lstat(items[i].dst)
		switch {
		case err == nil:
			if err := renamePath(items[i].dst, prior); err != nil {
				return abortStaged(ctx, items, i, errors.Join(fmt.Errorf("backup: %w", err), os.Remove(prior)))
			}
			items[i].prior = prior
		case errors.Is(err, fs.ErrNotExist):
		default:
			return abortStaged(ctx, items, i, errors.Join(fmt.Errorf("backup: %w", err), os.Remove(prior)))
		}
		if err := renamePath(items[i].fresh, items[i].dst); err != nil {
			return abortFreshRename(ctx, items, i, err)
		}
		items[i].placed = true
		items[i].fresh = ""
	}
	return nil
}

func abortFreshRename(ctx context.Context, items []staged, i int, err error) error {
	freshErr := fmt.Errorf("backup: rename %s: %w", items[i].dst, err)
	if items[i].prior == "" {
		return abortStaged(ctx, items, i, freshErr)
	}
	backErr := renamePath(items[i].prior, items[i].dst)
	if backErr == nil {
		items[i].prior = ""
		return abortStaged(ctx, items, i, freshErr)
	}
	kept := fmt.Errorf(
		"backup: %s is absent; pre-restore copy remains at %s; backup copy remains at %s: %w",
		items[i].dst,
		items[i].prior,
		items[i].fresh,
		errors.Join(freshErr, fmt.Errorf("backup: %w", backErr)),
	)
	return abortKeepCurrent(ctx, items, i, kept)
}

func removePriors(items []staged) error {
	var rmErr error
	for _, item := range items {
		if item.prior == "" {
			continue
		}
		if err := os.RemoveAll(item.prior); err != nil {
			rmErr = errors.Join(rmErr, fmt.Errorf("backup: remove previous %s: %w", item.dst, err))
		}
	}
	return rmErr
}

func abortStaged(ctx context.Context, items []staged, n int, err error) error {
	if n < len(items) {
		err = errors.Join(err, discardFresh(items[n:]))
	}
	return undoStaged(ctx, items[:n], err)
}

func abortKeepCurrent(ctx context.Context, items []staged, n int, err error) error {
	if n+1 < len(items) {
		err = errors.Join(err, discardFresh(items[n+1:]))
	}
	return undoStaged(ctx, items[:n], err)
}

func undoStaged(ctx context.Context, done []staged, cause error) error {
	var back error
	for i := range slices.Backward(done) {
		if err := ctx.Err(); err != nil {
			back = errors.Join(back, remainingBackupCopies(done[:i+1]), fmt.Errorf("backup: %w", err))
			break
		}
		item := done[i]
		if !item.placed {
			back = errors.Join(back, discardFresh([]staged{item}))
			continue
		}
		held, err := spareName(filepath.Dir(item.dst))
		if err != nil {
			back = errors.Join(back, err)
			continue
		}
		if err := renamePath(item.dst, held); err != nil {
			back = errors.Join(back, fmt.Errorf("backup: %w", err))
			continue
		}
		if item.prior != "" {
			if err := renamePath(item.prior, item.dst); err != nil {
				back = errors.Join(back, retainCopies(item.dst, item.prior, held, err))
				break
			}
		}
		if err := os.RemoveAll(held); err != nil {
			back = errors.Join(back, fmt.Errorf("backup: %w", err))
		}
	}
	return errors.Join(cause, back)
}

func remainingBackupCopies(items []staged) error {
	var err error
	for _, item := range items {
		if !item.placed {
			continue
		}
		err = errors.Join(err, fmt.Errorf("backup: %s remains the backup copy", item.dst))
	}
	return err
}

func retainCopies(dst, prior, held string, renameErr error) error {
	wrapped := fmt.Errorf("backup: %w", renameErr)
	_, statErr := os.Lstat(dst)
	switch {
	case statErr == nil:
		return fmt.Errorf("backup: pre-restore copy remains at %s; backup copy remains at %s; %s was not restored: %w", prior, held, dst, wrapped)
	case errors.Is(statErr, fs.ErrNotExist):
		if err := renamePath(held, dst); err != nil {
			return fmt.Errorf("backup: %s is absent; pre-restore copy remains at %s; backup copy remains at %s: %w", dst, prior, held, errors.Join(wrapped, fmt.Errorf("backup: %w", err)))
		}
		return fmt.Errorf("backup: pre-restore copy remains at %s; backup copy remains at %s: %w", prior, dst, wrapped)
	default:
		return fmt.Errorf("backup: pre-restore copy remains at %s; backup copy remains at %s; %s was not restored: %w", prior, held, dst, errors.Join(wrapped, fmt.Errorf("backup: %w", statErr)))
	}
}

func discardFresh(items []staged) error {
	var err error
	for _, item := range items {
		if item.fresh == "" {
			continue
		}
		if rmErr := os.RemoveAll(item.fresh); rmErr != nil {
			err = errors.Join(err, fmt.Errorf("backup: %w", rmErr))
		}
	}
	return err
}

func spareName(dir string) (string, error) {
	f, err := os.CreateTemp(dir, ".wormswmd-old-")
	if err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	name := f.Name()
	err = errors.Join(f.Close(), os.Remove(name))
	if err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	return name, nil
}

func contained(app, path string) error {
	if err := safe.IntermediateEscape(app, path); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	return nil
}

func rejectEscapes(app string) error {
	paths := []string{
		game.Executable(app),
		filepath.Join(app, dirContents, dirFrameworks, "QtCore.framework"),
		filepath.Join(app, dirContents, dirPlugIns, "platforms"),
		filepath.Join(app, dirContents, fileInfoPlist),
		filepath.Join(app, dirContents, dirResources, dirDataOSX, "SteamConfig.txt"),
		filepath.Join(app, dirContents, dirResources, dirCommonData, "AnalyticsConfig.txt"),
		filepath.Join(app, dirContents, dirCodeSignature, "CodeResources"),
	}
	for _, path := range paths {
		if err := contained(app, path); err != nil {
			return err
		}
	}
	return nil
}

func rejectInsideApp(app, dest string) error {
	err := safe.InRoot(app, dest)
	if err == nil {
		return fmt.Errorf("backup: %s is inside %s", dest, app)
	}
	if !errors.Is(err, safe.ErrOutside) {
		return fmt.Errorf("backup: %w", err)
	}
	realApp, err := filepath.EvalSymlinks(app)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	existing := dest
	for {
		_, statErr := os.Lstat(existing)
		if statErr == nil {
			break
		}
		if !errors.Is(statErr, fs.ErrNotExist) {
			return fmt.Errorf("backup: %w", statErr)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return fmt.Errorf("backup: %s does not exist", dest)
		}
		existing = parent
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if err := safe.InRoot(realApp, resolved); err == nil {
		return fmt.Errorf("backup: %s is inside %s", dest, app)
	}
	return nil
}

func removeCreated(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("backup: remove %s: %w", dir, err)
	}
	return nil
}

func moveAside(ctx context.Context, app, dst string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	if err := contained(app, dst); err != nil {
		return "", err
	}
	_, err := os.Lstat(dst)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("backup: %w", err)
	}
	spare, err := spareName(filepath.Dir(dst))
	if err != nil {
		return "", err
	}
	if err := renamePath(dst, spare); err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	return spare, nil
}

func loadMetadata(dir string) (metadata, error) {
	data, err := readRegular(filepath.Join(dir, metadataName))
	if err != nil {
		return metadata{}, err
	}
	return parseMetadata(data)
}

func readRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("backup: %s is not a regular file", path)
	}
	n, err := linkCount(info)
	if err != nil {
		return nil, err
	}
	if n > maxNlink {
		return nil, fmt.Errorf("backup: hard link %s has more than %d link; copy the file so it has one link, then retry", path, maxNlink)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}
	return data, nil
}

func requireKind(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("backup: %s is a symlink", path)
	}
	if directory {
		if !info.IsDir() {
			return fmt.Errorf("backup: %s is not a directory", path)
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("backup: %s is not a regular file", path)
	}
	return nil
}

func isDir(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("backup: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("backup: %s is a symlink", path)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("backup: %s is not a directory", path)
	}
	return true, nil
}

func hashFile(ctx context.Context, path string) (string, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, fmt.Errorf("backup: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("backup: %w", err)
	}
	h := sha256.New()
	n, copyErr := io.Copy(h, f)
	closeErr := f.Close()
	if copyErr != nil {
		return "", 0, fmt.Errorf("backup: %w", copyErr)
	}
	if closeErr != nil {
		return "", 0, fmt.Errorf("backup: %w", closeErr)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func hashText(text string) (string, int64) {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:]), int64(len(text))
}

func linkCount(info fs.FileInfo) (uint64, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("backup: stat is %T", info.Sys())
	}
	return uint64(stat.Nlink), nil //nolint:unconvert // Stat_t.Nlink is uint16 on darwin and uint64 on linux.
}

func executablePath(dir string) string {
	return filepath.Join(dir, executableRel())
}

func executableRel() string {
	return filepath.ToSlash(filepath.Join(dirMacOS, filepath.Base(game.Executable("."))))
}

func writePrivate(path, data string) error {
	if err := os.WriteFile(path, []byte(data), backupFileMode); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	return nil
}
