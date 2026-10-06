package qt

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/openbunny/wormswmd/internal/bundle"
	"github.com/openbunny/wormswmd/internal/plistfix"
	"github.com/openbunny/wormswmd/internal/safe"
)

const (
	ArchiveName       = "qt-frameworks-x86_64-5.15.19.tar.gz"
	PinSHA256         = "e16e16c165a4e2b3ea37757b2a566ddaf53f9e68a193886d868d1a7e1680a718"
	DistCommit        = "e62705a7445d28ea048159a858878b0f787dfd13"
	ArchiveURL        = "https://raw.githubusercontent.com/cboyd0319/WormsWMD-macOS-Fix/" + DistCommit + "/dist/" + ArchiveName
	Series            = "5.15"
	shortPrefix       = Series + "."
	fetchTimeout      = 5 * time.Minute
	fetchAttempts     = 2
	maxDownloadBytes  = 64 << 20
	maxExtractBytes   = 512 << 20
	maxExtractFiles   = 100000
	sha256HexLen      = 64
	dirMode           = 0o755
	fileMode          = 0o644
	serverStatusError = 500
	coreName          = "QtCore"
	qtPrefix          = "Qt"
	sharpyuvDylib     = "libsharpyuv.0.dylib"
	qtPluginPrefix    = "libq"
	webpPrefix        = "libwebp"
)

var (
	errChecksum = errors.New("qt: checksum mismatch")
	httpClient  = &http.Client{Timeout: fetchTimeout}
	extraFrames = []string{"QtDBus", "QtSvg"}
	fallback    = []string{"QtCore", "QtGui", "QtWidgets", "QtOpenGL", "QtPrintSupport"}
)

type statusError struct {
	Code int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("qt: HTTP status %d", e.Code)
}

func NormalizePin(pin string) (string, error) {
	pin = strings.ToLower(strings.TrimSpace(pin))
	if len(pin) != sha256HexLen {
		return "", fmt.Errorf("qt: SHA-256 pin must be %d hex characters", sha256HexLen)
	}
	if _, err := hex.DecodeString(pin); err != nil {
		return "", fmt.Errorf("qt: SHA-256 pin: %w", err)
	}
	return pin, nil
}

func SameVersion(version string) bool {
	return version == Series || strings.HasPrefix(version, shortPrefix)
}

var openFile = func(name string) (fs.File, error) { return os.Open(name) }

func VerifyFile(ctx context.Context, path, pin string) (err error) {
	pin, err = NormalizePin(pin)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	f, err := openFile(path)
	if err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("qt: close %s: %w", path, cerr))
		}
	}()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return fmt.Errorf("qt: hash %s: %w", path, err)
	}
	got := hex.EncodeToString(sum.Sum(nil))
	if got != pin {
		return fmt.Errorf("qt: SHA-256 of %s is %s; expected %s: %w", path, got, pin, errChecksum)
	}
	return nil
}

func Fetch(ctx context.Context, dest, url, pin string) error {
	pin, err := NormalizePin(pin)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	if _, err := os.Stat(dest); err == nil {
		if err := VerifyFile(ctx, dest, pin); err == nil {
			return nil
		} else if !errors.Is(err, errChecksum) {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("qt: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), dirMode); err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	var last error
	for attempt := range fetchAttempts {
		attemptCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
		last = fetchOnce(attemptCtx, dest, url, pin)
		cancel()
		if last == nil {
			return nil
		}
		if !transient(last) || ctx.Err() != nil || attempt+1 == fetchAttempts {
			return last
		}
		slog.Warn("qt: fetch attempt failed", "attempt", attempt+1, "err", last)
	}
	return last
}

func fetchOnce(ctx context.Context, dest, url, pin string) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("qt: close response: %w", cerr))
		}
	}()
	if resp.StatusCode != http.StatusOK {
		if _, drainErr := io.Copy(io.Discard, io.LimitReader(resp.Body, maxDownloadBytes)); drainErr != nil {
			return errors.Join(&statusError{Code: resp.StatusCode}, fmt.Errorf("qt: drain response: %w", drainErr))
		}
		return &statusError{Code: resp.StatusCode}
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".qt-*.partial")
	if err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if ok {
			return
		}
		if rmErr := os.Remove(tmpName); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("qt: remove %s: %w", tmpName, rmErr))
		}
	}()
	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, sum), io.LimitReader(resp.Body, maxDownloadBytes+1))
	closeErr := tmp.Close()
	if err != nil {
		return fmt.Errorf("qt: download: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("qt: %w", closeErr)
	}
	if n > maxDownloadBytes {
		return fmt.Errorf("qt: download exceeds %d bytes", maxDownloadBytes)
	}
	got := hex.EncodeToString(sum.Sum(nil))
	if got != pin {
		return fmt.Errorf("qt: SHA-256 is %s; expected %s: %w", got, pin, errChecksum)
	}
	if err := os.Chmod(tmpName, fileMode); err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	ok = true
	return nil
}

func transient(err error) bool {
	if errors.Is(err, errChecksum) || errors.Is(err, context.Canceled) {
		return false
	}
	if status, ok := errors.AsType[*statusError](err); ok {
		return status.Code >= serverStatusError
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func ExtractFile(ctx context.Context, path, dest string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	extractErr := Extract(ctx, f, dest)
	closeErr := f.Close()
	if extractErr != nil {
		return extractErr
	}
	if closeErr != nil {
		return fmt.Errorf("qt: %w", closeErr)
	}
	return nil
}

func Extract(ctx context.Context, r io.Reader, dest string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	zr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("qt: gzip: %w", err)
	}
	err = extractTar(ctx, zr, dest)
	closeErr := zr.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return fmt.Errorf("qt: gzip: %w", closeErr)
	}
	return nil
}

func extractTar(ctx context.Context, r io.Reader, dest string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	if err := os.MkdirAll(dest, dirMode); err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	tr := tar.NewReader(r)
	var files int
	var bytes int64
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("qt: %w", err)
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("qt: tar: %w", err)
		}
		files++
		if files > maxExtractFiles {
			return fmt.Errorf("qt: tar exceeds %d files", maxExtractFiles)
		}
		name, err := safe.CleanRel(hdr.Name)
		if err != nil {
			return fmt.Errorf("qt: %s: %w", hdr.Name, err)
		}
		if name == "." {
			continue
		}
		target := filepath.Join(dest, name)
		if err := safe.InRoot(dest, target); err != nil {
			return fmt.Errorf("qt: %w", err)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, dirMode); err != nil {
				return fmt.Errorf("qt: %w", err)
			}
		case tar.TypeReg:
			if hdr.Size < 0 || hdr.Size > maxExtractBytes || bytes > maxExtractBytes-hdr.Size {
				return fmt.Errorf("qt: tar exceeds %d bytes", maxExtractBytes)
			}
			if err := writeReg(ctx, tr, target, hdr); err != nil {
				return err
			}
			bytes += hdr.Size
		case tar.TypeSymlink:
			if err := safeSymlink(dest, target, hdr.Linkname); err != nil {
				return fmt.Errorf("qt: %s: %w", name, err)
			}
			if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
				return fmt.Errorf("qt: %w", err)
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return fmt.Errorf("qt: symlink %s: %w", name, err)
			}
		default:
			return fmt.Errorf("qt: unsupported tar entry %s type %d", name, hdr.Typeflag)
		}
	}
}

func writeReg(ctx context.Context, r io.Reader, target string, hdr *tar.Header) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode).Perm())
	if err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	n, copyErr := io.Copy(out, io.LimitReader(r, hdr.Size))
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("qt: %s: %w", target, errors.Join(copyErr, closeErr))
	}
	if closeErr != nil {
		return fmt.Errorf("qt: %s: %w", target, closeErr)
	}
	if n != hdr.Size {
		return fmt.Errorf("qt: %s: read %d bytes; header size is %d", target, n, hdr.Size)
	}
	return nil
}

func safeSymlink(root, linkPath, target string) error {
	if err := safe.ControlChars(target, "symlink target"); err != nil {
		return err
	}
	if filepath.IsAbs(target) {
		return fmt.Errorf("absolute symlink target %s", target)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(linkPath), target))
	return safe.InRoot(root, resolved)
}

func Validate(ctx context.Context, prefix string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	core := filepath.Join(prefix, "Frameworks", coreName+".framework")
	if _, err := bundle.Binary(core, coreName); err != nil {
		return fmt.Errorf("qt: %w", err)
	}
	version, err := shortVersion(ctx, core)
	if err != nil {
		return err
	}
	if !SameVersion(version) {
		return fmt.Errorf("qt: QtCore version is %q; expected %s", version, shortPrefix+"x")
	}
	for _, rel := range []string{
		filepath.Join("PlugIns", "platforms", "libqcocoa.dylib"),
		filepath.Join("PlugIns", "imageformats", "libqsvg.dylib"),
	} {
		path := filepath.Join(prefix, rel)
		st, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("qt: %s: %w", path, err)
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("qt: %s is not a regular file", path)
		}
	}
	return nil
}

func shortVersion(ctx context.Context, core string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("qt: %w", err)
	}
	bin, err := bundle.Binary(core, coreName)
	if err != nil {
		return "", fmt.Errorf("qt: %w", err)
	}
	path := filepath.Join(filepath.Dir(bin), "Resources", "Info.plist")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("qt: %s: %w", path, err)
	}
	version, err := plistfix.ShortVersion(data)
	if err != nil {
		return "", err
	}
	if version == "" {
		return "", fmt.Errorf("qt: %s has no %s", path, plistfix.KeyVersion)
	}
	return version, nil
}

func Frameworks(ctx context.Context, gameFrameworks, prefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("qt: %w", err)
	}
	entries, err := os.ReadDir(gameFrameworks)
	if err != nil {
		return nil, fmt.Errorf("qt: %w", err)
	}
	set := map[string]struct{}{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() && strings.HasPrefix(name, qtPrefix) && strings.HasSuffix(name, ".framework") {
			set[strings.TrimSuffix(name, ".framework")] = struct{}{}
		}
	}
	if len(set) == 0 {
		for _, name := range fallback {
			set[name] = struct{}{}
		}
	}
	for _, name := range extraFrames {
		set[name] = struct{}{}
	}
	names := slices.Sorted(maps.Keys(set))
	for _, name := range names {
		dir := filepath.Join(prefix, "Frameworks", name+".framework")
		if _, err := bundle.Binary(dir, name); err != nil {
			return nil, fmt.Errorf("qt: %w", err)
		}
	}
	return names, nil
}

func DependencyDylibs(ctx context.Context, prefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("qt: %w", err)
	}
	dir := filepath.Join(prefix, "Frameworks")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("qt: %w", err)
	}
	haveSharpyuv := false
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if name == sharpyuvDylib {
			haveSharpyuv = true
		}
		if !strings.HasSuffix(name, ".dylib") || strings.HasPrefix(name, qtPluginPrefix) {
			continue
		}
		names = append(names, name)
	}
	if !haveSharpyuv {
		names = slices.DeleteFunc(names, func(name string) bool {
			return strings.HasPrefix(name, webpPrefix)
		})
	}
	slices.Sort(names)
	return names, nil
}
