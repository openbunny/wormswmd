package tree

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/openbunny/wormswmd/internal/safe"
)

const (
	dirMode    = 0o755
	ownerWrite = 0o200
)

func Copy(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("tree: %w", err)
	}
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("tree: stat %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), dirMode); err != nil {
		return fmt.Errorf("tree: mkdir %s: %w", filepath.Dir(dst), err)
	}
	return copyEntry(ctx, src, dst, dst, info)
}

func copyEntry(ctx context.Context, src, dst, root string, info fs.FileInfo) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("tree: %w", err)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return fmt.Errorf("tree: readlink %s: %w", src, err)
		}
		if err := safeLink(root, dst, target); err != nil {
			return fmt.Errorf("tree: symlink %s: %w", src, err)
		}
		if err := os.Symlink(target, dst); err != nil {
			return fmt.Errorf("tree: symlink %s: %w", dst, err)
		}
		return nil
	case info.IsDir():
		if err := os.MkdirAll(dst, dirMode); err != nil {
			return fmt.Errorf("tree: mkdir %s: %w", dst, err)
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return fmt.Errorf("tree: read %s: %w", src, err)
		}
		for _, entry := range entries {
			childInfo, err := entry.Info()
			if err != nil {
				return fmt.Errorf("tree: stat %s: %w", entry.Name(), err)
			}
			if err := copyEntry(ctx, filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name()), root, childInfo); err != nil {
				return err
			}
		}
		return nil
	case info.Mode().IsRegular():
		return copyFile(ctx, src, dst, info.Mode())
	default:
		return fmt.Errorf("tree: unsupported file type: %s", src)
	}
}

func copyFile(ctx context.Context, src, dst string, mode fs.FileMode) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("tree: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), dirMode); err != nil {
		return fmt.Errorf("tree: mkdir %s: %w", filepath.Dir(dst), err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("tree: open %s: %w", src, err)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm()|ownerWrite)
	if err != nil {
		return fmt.Errorf("tree: create %s: %w", dst, errors.Join(err, in.Close()))
	}
	_, copyErr := io.Copy(out, in)
	inErr := in.Close()
	outErr := out.Close()
	switch {
	case copyErr != nil:
		return fmt.Errorf("tree: copy %s: %w", src, errors.Join(copyErr, inErr, outErr))
	case inErr != nil:
		return fmt.Errorf("tree: close %s: %w", src, errors.Join(inErr, outErr))
	case outErr != nil:
		return fmt.Errorf("tree: close %s: %w", dst, outErr)
	}
	return nil
}

func safeLink(root, linkPath, target string) error {
	if err := safe.ControlChars(target, "symlink target"); err != nil {
		return err
	}
	if filepath.IsAbs(target) {
		return fmt.Errorf("absolute symlink target %s", target)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(linkPath), target))
	return safe.InRoot(root, resolved)
}
