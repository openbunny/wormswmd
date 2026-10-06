package game

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openbunny/wormswmd/internal/safe"
)

const (
	execName    = "Worms W.M.D"
	bundleName  = "Worms W.M.D.app"
	altBundle   = "Worms WMD.app"
	SteamAppID  = "327030"
	steamMarker = "libsteam_api.dylib"
	gogMarker   = "libGalaxy.dylib"
	walkDepth   = 2

	defaultApplications = "/Applications"
)

var (
	ErrNotBundle = errors.New("not a bundle")
	ErrNotFound  = errors.New("not found")
	vdfPath      = regexp.MustCompile(`"path"\s+"([^"]+)"`)
)

type AmbiguousError struct {
	Paths []string
}

func (e *AmbiguousError) Error() string {
	return "several Worms W.M.D.app bundles match; pass --app:\n  " + strings.Join(e.Paths, "\n  ")
}

func steamSupport(home string, rest ...string) string {
	return filepath.Join(append([]string{home, "Library", "Application Support", "Steam"}, rest...)...)
}

func Executable(app string) string {
	return filepath.Join(app, "Contents", "MacOS", execName)
}

func Valid(ctx context.Context, app string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("game: %w", err)
	}
	if err := safe.ControlChars(app, "game"); err != nil {
		return fmt.Errorf("game: %w", errors.Join(ErrNotBundle, err))
	}
	info, err := os.Lstat(app)
	if err != nil {
		return fmt.Errorf("game: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("game bundle is a symlink: %s: %w", app, ErrNotBundle)
	}
	if !info.IsDir() {
		return fmt.Errorf("game is not a directory: %s: %w", app, ErrNotBundle)
	}
	exe := Executable(app)
	st, err := os.Lstat(exe)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("game executable: %w", ErrNotBundle)
		}
		return fmt.Errorf("game: %w", err)
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("game executable is not a regular file: %s: %w", exe, ErrNotBundle)
	}
	return nil
}

func Source(ctx context.Context, app string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("game: %w", err)
	}
	steam, err := markerExists(ctx, filepath.Join(app, "Contents", "Frameworks", steamMarker))
	if err != nil {
		return "", err
	}
	if steam {
		return "steam", nil
	}
	gog, err := markerExists(ctx, filepath.Join(app, "Contents", "MacOS", gogMarker))
	if err != nil {
		return "", err
	}
	if gog {
		return "gog", nil
	}
	lower := strings.ToLower(app)
	switch {
	case strings.Contains(lower, "steamapps"):
		return "steam", nil
	case strings.Contains(lower, "gog"):
		return "gog", nil
	default:
		return "unknown", nil
	}
}

func markerExists(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("game: %w", err)
	}
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("game: %w", err)
}

func Candidates(ctx context.Context, home, applications string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("game: %w", err)
	}
	if applications == "" {
		applications = defaultApplications
	}
	list := []string{
		filepath.Join(steamSupport(home), "steamapps", "common", "WormsWMD", bundleName),
		filepath.Join(applications, bundleName),
		filepath.Join(applications, altBundle),
		filepath.Join(home, "Applications", bundleName),
		filepath.Join(home, "Applications", altBundle),
		filepath.Join(home, "Games", bundleName),
		filepath.Join(home, "Games", altBundle),
		filepath.Join(home, "GOG Games", "Worms W.M.D", bundleName),
		filepath.Join(home, "GOG Games", bundleName),
		filepath.Join(home, "Library", "Application Support", "GOG.com", "Games", "Worms W.M.D", bundleName),
	}
	for _, root := range []string{
		applications,
		filepath.Join(home, "Applications"),
		filepath.Join(home, "Games"),
		filepath.Join(home, "GOG Games"),
		filepath.Join(home, "Library", "Application Support", "GOG.com", "Games"),
	} {
		found, err := shallowBundles(ctx, root)
		if err != nil {
			return nil, err
		}
		list = append(list, found...)
	}
	steam, err := steamLibraryApps(ctx, home)
	if err != nil {
		return nil, err
	}
	return append(list, steam...), nil
}

func Find(ctx context.Context, home, applications string) ([]string, error) {
	candidates, err := Candidates(ctx, home, applications)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var found []string
	for _, path := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("game: %w", err)
		}
		if err := Valid(ctx, path); err != nil {
			if errors.Is(err, ErrNotBundle) || errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		key, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, fmt.Errorf("game: %s: %w", path, err)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		found = append(found, path)
	}
	return found, nil
}

func Resolve(ctx context.Context, explicit, home, applications string) (string, error) {
	if explicit != "" {
		if err := Valid(ctx, explicit); err != nil {
			return "", err
		}
		return explicit, nil
	}
	found, err := Find(ctx, home, applications)
	if err != nil {
		return "", err
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("game: Worms W.M.D.app %w; pass --app", ErrNotFound)
	case 1:
		return found[0], nil
	default:
		return "", &AmbiguousError{Paths: found}
	}
}

func SteamSaveDirs(ctx context.Context, home string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("game: %w", err)
	}
	root := steamSupport(home, "userdata")
	info, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("steam userdata: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("steam userdata is not a directory: %s", root)
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("steam userdata: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read steam userdata: %w", err)
	}
	var saves []string
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("game: %w", err)
		}
		id := entry.Name()
		if strings.Contains(id, ".") {
			continue
		}
		dir := filepath.Join(root, id, SteamAppID)
		st, err := os.Lstat(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("steam save %s: %w", id, err)
		}
		if !st.IsDir() && st.Mode()&os.ModeSymlink == 0 {
			continue
		}
		resolvedPath, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return nil, fmt.Errorf("steam save %s: %w", id, err)
		}
		if err := safe.InRoot(rootReal, resolvedPath); err != nil {
			return nil, fmt.Errorf("steam save %s leaves userdata: %w", id, err)
		}
		if st.Mode()&os.ModeSymlink != 0 {
			target, err := os.Stat(resolvedPath)
			if err != nil {
				return nil, fmt.Errorf("steam save %s: %w", id, err)
			}
			if !target.IsDir() {
				continue
			}
		}
		saves = append(saves, dir)
	}
	return saves, nil
}

func Team17Dir(ctx context.Context, home string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("game: %w", err)
	}
	dir := filepath.Join(home, "Library", "Application Support", "Team17")
	info, err := os.Lstat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("Team17 saves: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("Team17 saves are not a directory: %s", dir)
	}
	return dir, nil
}

func LibraryPaths(data string) []string {
	var paths []string
	for _, m := range vdfPath.FindAllStringSubmatch(data, -1) {
		path := strings.ReplaceAll(m[1], `\\`, `\`)
		if safe.ControlChars(path, "steam library") != nil {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func shallowBundles(ctx context.Context, root string) ([]string, error) {
	info, err := os.Lstat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("game: %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, nil
	}
	var found []string
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("game: %s: %w", path, err)
		}
		depth := 0
		if rel != "." {
			depth = strings.Count(rel, string(filepath.Separator)) + 1
		}
		if depth > walkDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() && (name == bundleName || name == altBundle) && path != root {
			found = append(found, path)
			return filepath.SkipDir
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("game: %s: %w", root, walkErr)
	}
	return found, nil
}

func steamLibraryApps(ctx context.Context, home string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("game: %w", err)
	}
	path := steamSupport(home, "steamapps", "libraryfolders.vdf")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("game: %s: %w", path, err)
	}
	var apps []string
	for _, lib := range LibraryPaths(string(data)) {
		apps = append(apps, filepath.Join(lib, "steamapps", "common", "WormsWMD", bundleName))
	}
	return apps, nil
}
