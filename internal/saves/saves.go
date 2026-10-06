package saves

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/openbunny/wormswmd/internal/game"
	"github.com/openbunny/wormswmd/internal/safe"
	"github.com/openbunny/wormswmd/internal/tree"
)

const (
	backupRootName = "WormsWMD-SaveBackups"
	stampLayout    = "WormsWMD-SaveBackup-20060102-150405"
	dirMode        = 0o700
	sharedDirMode  = 0o755
	teamDirName    = "Team17"
	steamDirName   = "Steam"
	absentSaves    = "no Worms saves found"
)

func Backup(ctx context.Context, home string, now time.Time) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("saves: %w", err)
	}
	team, err := game.Team17Dir(ctx, home)
	if err != nil {
		return "", fmt.Errorf("saves: %w", err)
	}
	steam, err := game.SteamSaveDirs(ctx, home)
	if err != nil {
		return "", fmt.Errorf("saves: %w", err)
	}
	if team == "" && len(steam) == 0 {
		return "", fmt.Errorf("saves: %s", absentSaves)
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	root := backupRoot(home)
	if err := ensureBackupRoot(ctx, root); err != nil {
		return "", err
	}
	dest := filepath.Join(root, now.Format(stampLayout))
	if err := os.Mkdir(dest, dirMode); err != nil {
		return "", fmt.Errorf("saves: %w", err)
	}
	if err := os.Chmod(dest, dirMode); err != nil {
		return "", fmt.Errorf("saves: %w", err)
	}
	if team != "" {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("saves: %w", err)
		}
		if err := tree.Copy(ctx, team, filepath.Join(dest, teamDirName)); err != nil {
			return "", fmt.Errorf("saves: %w", err)
		}
	}
	for _, dir := range steam {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("saves: %w", err)
		}
		id := filepath.Base(filepath.Dir(dir))
		if err := tree.Copy(ctx, dir, filepath.Join(dest, steamDirName, id)); err != nil {
			return "", fmt.Errorf("saves: %w", err)
		}
	}
	return dest, nil
}

func Restore(ctx context.Context, home, dir string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("saves: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("saves: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("saves: backup %s is not a directory", dir)
	}
	teamSrc := filepath.Join(dir, teamDirName)
	teamDest := filepath.Join(home, "Library", "Application Support", teamDirName)
	restoreTeam, err := backupDir(ctx, teamSrc)
	if err != nil {
		return fmt.Errorf("saves: %w", err)
	}
	if restoreTeam {
		if err := refuseTeamSymlink(teamDest); err != nil {
			return err
		}
		if err := safe.InRoot(home, teamDest); err != nil {
			return fmt.Errorf("saves: %w", err)
		}
	}
	ids, err := steamBackupIDs(ctx, dir)
	if err != nil {
		return fmt.Errorf("saves: %w", err)
	}
	userdata := filepath.Join(home, "Library", "Application Support", "Steam", "userdata")
	dests := make([]string, 0, len(ids))
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("saves: %w", err)
		}
		dest := filepath.Join(userdata, id, game.SteamAppID)
		if err := steamStaysInside(userdata, dest); err != nil {
			return err
		}
		dests = append(dests, dest)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("saves: %w", err)
	}
	if !restoreTeam && len(ids) == 0 {
		return fmt.Errorf("saves: backup %s has no saves", dir)
	}
	if restoreTeam {
		if err := replaceDir(ctx, teamSrc, teamDest); err != nil {
			return err
		}
	}
	for i, id := range ids {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("saves: %w", err)
		}
		src := filepath.Join(dir, steamDirName, id)
		if err := replaceDir(ctx, src, dests[i]); err != nil {
			return err
		}
	}
	return nil
}

func List(ctx context.Context, home string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("saves: %w", err)
	}
	root := backupRoot(home)
	info, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("saves: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("saves: backup root %s is a symlink, want a directory", root)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("saves: backup root %s is not a directory", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("saves: %w", err)
	}
	names := []string{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("saves: %w", err)
		}
		if entry.IsDir() {
			names = append(names, filepath.Join(root, entry.Name()))
		}
	}
	slices.Sort(names)
	return names, nil
}

func backupRoot(home string) string {
	return filepath.Join(home, "Documents", backupRootName)
}

func ensureBackupRoot(ctx context.Context, root string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("saves: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("saves: %w", err)
		}
		if err := os.MkdirAll(root, dirMode); err != nil {
			return fmt.Errorf("saves: %w", err)
		}
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("saves: backup root %s is a symlink, want a directory", root)
	}
	if !info.IsDir() {
		return fmt.Errorf("saves: backup root %s is not a directory", root)
	}
	return nil
}

func backupDir(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("saves: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s is not a directory", path)
	}
	return true, nil
}

func refuseTeamSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("saves: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("saves: Team17 destination %s is a symlink, want a directory", path)
	}
	return nil
}

func steamBackupIDs(ctx context.Context, dir string) ([]string, error) {
	root := filepath.Join(dir, steamDirName)
	info, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			return nil, fmt.Errorf("%s is not a directory", filepath.Join(root, entry.Name()))
		}
		ids = append(ids, entry.Name())
	}
	slices.Sort(ids)
	return ids, nil
}

func steamStaysInside(userdata, dest string) error {
	rootReal, err := filepath.EvalSymlinks(userdata)
	if err != nil {
		return fmt.Errorf("saves: %s leaves userdata: %w", dest, err)
	}
	_, err = os.Lstat(dest)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("saves: %s leaves userdata: %w", dest, err)
	}
	if err == nil {
		resolvedPath, err := filepath.EvalSymlinks(dest)
		if err != nil {
			return fmt.Errorf("saves: %s leaves userdata: %w", dest, err)
		}
		if err := safe.InRoot(rootReal, resolvedPath); err != nil {
			return fmt.Errorf("saves: %s leaves userdata: %w", dest, err)
		}
		return nil
	}
	parent, rest, err := nearestParent(dest)
	if err != nil {
		return fmt.Errorf("saves: %s leaves userdata: %w", dest, err)
	}
	parentReal, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return fmt.Errorf("saves: %s leaves userdata: %w", dest, err)
	}
	if err := safe.InRoot(rootReal, parentReal); err != nil {
		return fmt.Errorf("saves: %s leaves userdata: %w", dest, err)
	}
	joined := parentReal
	for _, name := range rest {
		joined = filepath.Join(joined, name)
	}
	if err := safe.InRoot(rootReal, joined); err != nil {
		return fmt.Errorf("saves: %s leaves userdata: %w", dest, err)
	}
	return nil
}

func nearestParent(path string) (string, []string, error) {
	var rest []string
	cur := path
	for {
		_, err := os.Lstat(cur)
		if err == nil {
			return cur, rest, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", nil, err
		}
		next := filepath.Dir(cur)
		if next == cur {
			return "", nil, fmt.Errorf("no existing parent of %s", path)
		}
		rest = append([]string{filepath.Base(cur)}, rest...)
		cur = next
	}
}

func replaceDir(ctx context.Context, src, dest string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("saves: %w", err)
	}
	parent := filepath.Dir(dest)
	base := filepath.Base(dest)
	if err := os.MkdirAll(parent, sharedDirMode); err != nil {
		return fmt.Errorf("saves: %w", err)
	}
	staged := filepath.Join(parent, ".wormswmd-new-"+base)
	prior := filepath.Join(parent, ".wormswmd-old-"+base)
	if err := os.RemoveAll(staged); err != nil {
		return fmt.Errorf("saves: %w", err)
	}
	if err := tree.Copy(ctx, src, staged); err != nil {
		if rmErr := os.RemoveAll(staged); rmErr != nil {
			return fmt.Errorf("saves: copy %s: %w; remove %s: %w", src, err, staged, rmErr)
		}
		return fmt.Errorf("saves: %w", err)
	}
	had := false
	if _, err := os.Lstat(dest); err == nil {
		had = true
		if err := os.RemoveAll(prior); err != nil {
			if rmErr := os.RemoveAll(staged); rmErr != nil {
				return fmt.Errorf("saves: %w; remove %s: %w", err, staged, rmErr)
			}
			return fmt.Errorf("saves: %w", err)
		}
		if err := os.Rename(dest, prior); err != nil {
			if rmErr := os.RemoveAll(staged); rmErr != nil {
				return fmt.Errorf("saves: rename %s: %w; remove %s: %w", dest, err, staged, rmErr)
			}
			return fmt.Errorf("saves: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		if rmErr := os.RemoveAll(staged); rmErr != nil {
			return fmt.Errorf("saves: stat %s: %w; remove %s: %w", dest, err, staged, rmErr)
		}
		return fmt.Errorf("saves: %w", err)
	}
	if err := os.Rename(staged, dest); err != nil {
		if had {
			if backErr := os.Rename(prior, dest); backErr != nil {
				return fmt.Errorf("saves: rename %s: %w; restore %s: %w", staged, err, dest, backErr)
			}
		}
		if rmErr := os.RemoveAll(staged); rmErr != nil {
			return fmt.Errorf("saves: rename %s: %w; remove %s: %w", staged, err, staged, rmErr)
		}
		return fmt.Errorf("saves: %w", err)
	}
	if !had {
		return nil
	}
	if err := os.RemoveAll(prior); err != nil {
		return fmt.Errorf("saves: replacement is at %s; previous tree remains at %s: %w", dest, prior, err)
	}
	return nil
}
