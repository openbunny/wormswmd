package safe

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

var ErrOutside = errors.New("outside")

func ControlChars(path, name string) error {
	if path == "" {
		return fmt.Errorf("%s is empty; expected a path", name)
	}
	if strings.IndexFunc(path, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
		return fmt.Errorf("%s contains a control character", name)
	}
	return nil
}

func CleanRel(path string) (string, error) {
	if err := ControlChars(path, "path"); err != nil {
		return "", err
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path is absolute: %s", path)
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes its root: %s", path)
	}
	return clean, nil
}

func InRoot(root, path string) error {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("path %s relative to %s: %w", path, root, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is outside %s: %w", path, root, ErrOutside)
	}
	return nil
}

func LinkEscape(root, linkPath, target string) error {
	if err := ControlChars(target, "symlink target"); err != nil {
		return err
	}
	if filepath.IsAbs(target) {
		return fmt.Errorf("absolute symlink target %s", target)
	}
	named := false
	for part := range strings.SplitSeq(target, "/") {
		switch {
		case part == "..":
			if named {
				return fmt.Errorf("symlink target %s has .. after a named component", target)
			}
		case part != "" && part != ".":
			named = true
		}
	}
	if err := IntermediateEscape(root, linkPath); err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("safe: resolve %s: %w", root, err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(linkPath))
	if err != nil {
		return fmt.Errorf("safe: resolve %s: %w", filepath.Dir(linkPath), err)
	}
	return InRoot(resolvedRoot, filepath.Join(parent, target))
}

func IntermediateEscape(root, path string) error {
	if err := InRoot(root, path); err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("safe: resolve %s: %w", root, err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("safe: %s relative to %s: %w", path, root, err)
	}
	parts := strings.Split(rel, string(filepath.Separator))
	cur := root
	for i, part := range parts {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("safe: lstat %s: %w", cur, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if i == len(parts)-1 {
			return nil
		}
		resolvedPath, err := filepath.EvalSymlinks(cur)
		if err != nil {
			return fmt.Errorf("safe: resolve %s: %w", cur, err)
		}
		if err := InRoot(resolved, resolvedPath); err != nil {
			return fmt.Errorf("%s leaves %s: %w", path, root, err)
		}
	}
	return nil
}
