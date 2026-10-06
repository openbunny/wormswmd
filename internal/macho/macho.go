package macho

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openbunny/wormswmd/internal/bundle"
	"github.com/openbunny/wormswmd/internal/game"
	"github.com/openbunny/wormswmd/internal/run"
	"github.com/openbunny/wormswmd/internal/safe"
)

const (
	execToken   = "@executable_path"
	loaderToken = "@loader_path"
	rpathToken  = "@rpath"
	AGLID       = execToken + "/../Frameworks/AGL.framework/AGL"
	usrLib      = "/usr/lib/"
	systemLib   = "/System/Library/"
	fwMarker    = ".framework/"
	userWrite   = 0o200
)

type Dep struct {
	Path string
	Weak bool
}

type Image struct {
	Path   string
	Deps   []Dep
	Rpaths []string
}

type Index struct {
	Contents   string
	Exec       string
	ExecRpaths []string
	Frameworks map[string]string
	Dylibs     map[string]string
}

type Edit struct {
	From string
	To   string
}

type PlanResult struct {
	ID       string
	Edits    []Edit
	Warnings []string
}

func Plan(img Image, id string, index Index, exists func(string) bool, inside func(string) bool) (PlanResult, error) {
	result := PlanResult{ID: id}
	for _, dep := range img.Deps {
		if hasToken(dep.Path, execToken) || hasToken(dep.Path, loaderToken) {
			resolved, ok := expandToken(dep.Path, img.Path, index.Exec)
			if !ok || !inside(resolved) {
				return PlanResult{}, fmt.Errorf("macho: %s: %s resolves outside the app bundle", img.Path, dep.Path)
			}
			if !exists(resolved) {
				if dep.Weak {
					result.Warnings = append(result.Warnings, fmt.Sprintf("Optional dependency %s is missing from %s; the load command is weak.", dep.Path, img.Path))
					continue
				}
				return PlanResult{}, fmt.Errorf("macho: %s: missing dependency %s; the file it names must exist inside the app; reinstall the game, then retry", img.Path, dep.Path)
			}
			continue
		}
		var next string
		if before, _, found := strings.Cut(dep.Path, fwMarker); found {
			next = index.Frameworks[filepath.Base(before)]
		} else {
			next = index.Dylibs[filepath.Base(dep.Path)]
		}
		if next != "" {
			if next != dep.Path {
				result.Edits = append(result.Edits, Edit{From: dep.Path, To: next})
			}
			continue
		}
		switch {
		case strings.HasPrefix(dep.Path, usrLib) || strings.HasPrefix(dep.Path, systemLib):
			continue
		case hasToken(dep.Path, rpathToken):
			if rpathInside(img, dep.Path, index, exists, inside) {
				continue
			}
			if dep.Weak {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Optional dependency %s is unresolved in %s; the load command is weak.", dep.Path, img.Path))
				continue
			}
			return PlanResult{}, fmt.Errorf("macho: %s: unresolved dependency %s; no LC_RPATH of the image or the game executable resolves it to a file inside the app; reinstall the game, then retry", img.Path, dep.Path)
		default:
			return PlanResult{}, fmt.Errorf("macho: %s: unportable dependency %s; a dependency must be a system library, an @executable_path, @loader_path or @rpath path, or a library the app bundles; report the file with wormswmd support", img.Path, dep.Path)
		}
	}
	return result, nil
}

func Rewrite(ctx context.Context, app string, exec run.Exec) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("macho: %w", err)
	}
	exec = run.Or(exec)
	gameExec := game.Executable(app)
	contents := filepath.Join(app, "Contents")
	frameworks := filepath.Join(contents, "Frameworks")
	plugins := filepath.Join(contents, "PlugIns")
	index := Index{Contents: contents, Exec: gameExec, Frameworks: map[string]string{}, Dylibs: map[string]string{}}
	var items []Image
	var ids []string
	add := func(path, id string) {
		items = append(items, Image{Path: path})
		ids = append(ids, id)
	}
	add(gameExec, "")
	fwEntries, err := os.ReadDir(frameworks)
	if err != nil {
		return nil, fmt.Errorf("macho: %w", err)
	}
	for _, entry := range fwEntries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".framework") {
			continue
		}
		fwDir := filepath.Join(frameworks, name)
		fwName := strings.TrimSuffix(name, ".framework")
		bin, err := bundle.Binary(fwDir, fwName)
		if errors.Is(err, bundle.ErrNoBinary) {
			continue
		}
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(fwDir, bin)
		if err != nil {
			return nil, fmt.Errorf("macho: %s: %w", bin, err)
		}
		id := execToken + "/../Frameworks/" + name + "/" + filepath.ToSlash(rel)
		if fwName == "AGL" {
			id = AGLID
		}
		index.Frameworks[fwName] = id
		add(bin, id)
		seen := map[string]bool{bin: true}
		walkErr := filepath.WalkDir(fwDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.Type().IsRegular() {
				return nil
			}
			if seen[path] || !machoFile(d.Name()) {
				return nil
			}
			seen[path] = true
			sibRel, err := filepath.Rel(fwDir, path)
			if err != nil {
				return err
			}
			add(path, execToken+"/../Frameworks/"+name+"/"+filepath.ToSlash(sibRel))
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("macho: %w", walkErr)
		}
	}
	for _, entry := range fwEntries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".dylib") {
			continue
		}
		path := filepath.Join(frameworks, name)
		id := execToken + "/../Frameworks/" + name
		index.Dylibs[name] = id
		add(path, id)
	}
	pluginErr := filepath.WalkDir(plugins, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || d.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(d.Name(), ".dylib") {
			return nil
		}
		rel, err := filepath.Rel(contents, path)
		if err != nil {
			return err
		}
		add(path, execToken+"/../"+filepath.ToSlash(rel))
		return nil
	})
	if pluginErr != nil {
		return nil, fmt.Errorf("macho: %w", pluginErr)
	}
	var statErr error
	noteStat := func(err error) {
		if err == nil || errors.Is(err, os.ErrNotExist) || statErr != nil {
			return
		}
		statErr = err
	}
	inside := func(path string) bool {
		resolvedPath, err := resolved(path)
		if errors.Is(err, os.ErrNotExist) {
			return false
		}
		if err != nil {
			noteStat(err)
			return true
		}
		return safe.InRoot(contents, resolvedPath) == nil
	}
	exists := func(path string) bool {
		st, err := os.Stat(path)
		if err != nil {
			noteStat(err)
			return false
		}
		return st.Mode().IsRegular()
	}
	var warnings []string
	for i, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("macho: %w", err)
		}
		deps, rpaths, err := loadImage(ctx, exec, item.Path)
		if err != nil {
			return nil, err
		}
		item.Deps = deps
		item.Rpaths = rpaths
		if item.Path == gameExec {
			index.ExecRpaths = rpaths
		}
		planned, err := Plan(item, ids[i], index, exists, inside)
		if statErr != nil {
			return nil, fmt.Errorf("macho: %w", statErr)
		}
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, planned.Warnings...)
		if err := applyPlan(ctx, exec, item.Path, planned); err != nil {
			return nil, err
		}
	}
	return warnings, nil
}

func machoFile(name string) bool {
	return strings.HasSuffix(name, ".dylib") || !strings.Contains(name, ".")
}

func loadImage(ctx context.Context, exec run.Exec, path string) ([]Dep, []string, error) {
	loads, err := otool(ctx, exec, "-L", path)
	if err != nil {
		return nil, nil, fmt.Errorf("macho: otool -L %s: %w", path, err)
	}
	dump, err := otool(ctx, exec, "-l", path)
	if err != nil {
		return nil, nil, fmt.Errorf("macho: otool -l %s: %w", path, err)
	}
	weak, rpaths, id := ParseLoads(string(dump))
	var deps []Dep
	for _, dep := range ParseDeps(string(loads)) {
		if dep == id {
			continue
		}
		deps = append(deps, Dep{Path: dep, Weak: weak[dep]})
	}
	return deps, rpaths, nil
}

func applyPlan(ctx context.Context, exec run.Exec, path string, plan PlanResult) error {
	if err := makeWritable(path); err != nil {
		return fmt.Errorf("macho: %s: %w", path, err)
	}
	if plan.ID != "" {
		if _, err := exec(ctx, "install_name_tool", "-id", plan.ID, path); err != nil {
			return fmt.Errorf("macho: install id %s: %w", path, err)
		}
	}
	for _, edit := range plan.Edits {
		if _, err := exec(ctx, "install_name_tool", "-change", edit.From, edit.To, path); err != nil {
			return fmt.Errorf("macho: change %s in %s: %w", edit.From, path, err)
		}
	}
	if len(plan.Edits) == 0 {
		return nil
	}
	loads, err := otool(ctx, exec, "-L", path)
	if err != nil {
		return fmt.Errorf("macho: otool -L %s: %w", path, err)
	}
	have := map[string]bool{}
	for _, dep := range ParseDeps(string(loads)) {
		have[dep] = true
	}
	for _, edit := range plan.Edits {
		if have[edit.From] || !have[edit.To] {
			return fmt.Errorf("macho: %s: install name %s was not changed to %s", path, edit.From, edit.To)
		}
	}
	return nil
}

func makeWritable(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if st.Mode()&userWrite != 0 {
		return nil
	}
	return os.Chmod(path, st.Mode().Perm()|userWrite)
}

func otool(ctx context.Context, exec run.Exec, flag, bin string) ([]byte, error) {
	out, err := exec(ctx, "otool", "-arch", "x86_64", flag, bin)
	if err == nil {
		return out, nil
	}
	if errorsCanceled(err) {
		return nil, err
	}
	return exec(ctx, "otool", flag, bin)
}

func errorsCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func ParseDeps(out string) []string {
	var deps []string
	for line := range strings.SplitSeq(out, "\n") {
		if line == "" || (line[0] != ' ' && line[0] != '\t') {
			continue
		}
		line = strings.TrimSpace(line)
		if i := strings.Index(line, " (compatibility version "); i >= 0 {
			line = line[:i]
		}
		if line != "" {
			deps = append(deps, line)
		}
	}
	return deps
}

func ParseLoads(out string) (weak map[string]bool, rpaths []string, id string) {
	weak = map[string]bool{}
	cmd := ""
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "cmd" {
			cmd = fields[1]
			continue
		}
		if len(fields) >= 2 && fields[0] == "name" && strings.Contains(cmd, "DYLIB") {
			switch cmd {
			case "LC_LOAD_WEAK_DYLIB":
				weak[stripLoad(line, "name")] = true
			case "LC_ID_DYLIB":
				id = stripLoad(line, "name")
			}
			cmd = ""
			continue
		}
		if len(fields) >= 2 && fields[0] == "path" && cmd == "LC_RPATH" {
			rpaths = append(rpaths, stripLoad(line, "path"))
			cmd = ""
		}
	}
	return weak, rpaths, id
}

func stripLoad(line, key string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, key)
	line = strings.TrimSpace(line)
	if i := strings.LastIndex(line, " (offset "); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

func hasToken(path, token string) bool {
	return path == token || strings.HasPrefix(path, token+"/")
}

func expandToken(path, loader, exec string) (string, bool) {
	switch {
	case path == execToken:
		return filepath.Dir(exec), true
	case strings.HasPrefix(path, execToken+"/"):
		return filepath.Join(filepath.Dir(exec), strings.TrimPrefix(path, execToken+"/")), true
	case path == loaderToken:
		return filepath.Dir(loader), true
	case strings.HasPrefix(path, loaderToken+"/"):
		return filepath.Join(filepath.Dir(loader), strings.TrimPrefix(path, loaderToken+"/")), true
	default:
		return "", false
	}
}

func rpathInside(img Image, dep string, index Index, exists func(string) bool, inside func(string) bool) bool {
	suffix := strings.TrimPrefix(dep, rpathToken+"/")
	bins := []string{img.Path}
	if img.Path != index.Exec {
		bins = append(bins, index.Exec)
	}
	for _, bin := range bins {
		var rpaths []string
		if bin == img.Path {
			rpaths = img.Rpaths
		} else {
			rpaths = index.ExecRpaths
		}
		for _, rpath := range rpaths {
			expanded, ok := expandToken(rpath, bin, index.Exec)
			if !ok {
				if !filepath.IsAbs(rpath) {
					continue
				}
				expanded = rpath
			}
			candidate := filepath.Join(expanded, suffix)
			if exists(candidate) && inside(candidate) {
				return true
			}
		}
	}
	return false
}

func resolved(path string) (string, error) {
	st, err := os.Lstat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	} else if st.Mode()&os.ModeSymlink != 0 || st.IsDir() {
		return filepath.EvalSymlinks(path)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}
