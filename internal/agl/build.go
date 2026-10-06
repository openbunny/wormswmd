package agl

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/openbunny/wormswmd/internal/progress"
	"github.com/openbunny/wormswmd/internal/run"
	"github.com/openbunny/wormswmd/internal/tree"
	"howett.net/plist"
)

//go:embed stub/agl_stub.c
var source string

//go:embed stub/agl_stub.h
var header string

const (
	installName    = "@executable_path/../Frameworks/AGL.framework/Versions/A/AGL"
	minX86         = "10.9"
	minARM         = "11.0"
	cStandard      = "c11"
	compatVersion  = "1.0.0"
	currentVersion = "1.0.0"
	bundleID       = "com.wormswmd.aglstub"
	archX86        = "x86_64"
	archARM        = "arm64"
	dirMode        = 0o755
	fileMode       = 0o644
	privateDir     = 0o700
	frameworkName  = "AGL.framework"
)

func Build(ctx context.Context, dest string, exec run.Exec) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("agl: %w", err)
	}
	exec = run.Or(exec)
	finish := progress.Begin("Building the AGL stub library the game needs")
	if err := os.MkdirAll(dest, privateDir); err != nil {
		return "", fmt.Errorf("agl: %w", err)
	}
	if err := os.Chmod(dest, privateDir); err != nil {
		return "", fmt.Errorf("agl: %w", err)
	}
	srcPath := filepath.Join(dest, "agl_stub.c")
	if err := os.WriteFile(srcPath, []byte(source), fileMode); err != nil {
		return "", fmt.Errorf("agl: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dest, "agl_stub.h"), []byte(header), fileMode); err != nil {
		return "", fmt.Errorf("agl: %w", err)
	}
	compilerOut, err := exec(ctx, "xcrun", "--toolchain", "default", "--sdk", "macosx", "--find", "clang")
	if err != nil {
		return "", fmt.Errorf("agl: clang is missing; install the Xcode Command Line Tools: xcode-select --install: %w", err)
	}
	sdkOut, err := exec(ctx, "xcrun", "--toolchain", "default", "--sdk", "macosx", "--show-sdk-path")
	if err != nil {
		return "", fmt.Errorf("agl: sdk: %w", err)
	}
	compiler := strings.TrimSpace(string(compilerOut))
	primary := strings.TrimSpace(string(sdkOut))
	candidates, err := sdkCandidates(primary)
	if err != nil {
		return "", err
	}
	var last error
	for _, sdk := range candidates {
		x86 := filepath.Join(dest, "AGL_x86_64")
		arm := filepath.Join(dest, "AGL_arm64")
		errX86 := compile(ctx, exec, compiler, sdk, archX86, minX86, srcPath, x86)
		errARM := compile(ctx, exec, compiler, sdk, archARM, minARM, srcPath, arm)
		if errX86 != nil || errARM != nil {
			last = errors.Join(errX86, errARM)
			slog.Debug("agl: SDK did not build both slices", "sdk", sdk, "err", last)
			continue
		}
		out := filepath.Join(dest, "AGL")
		if _, err := exec(ctx, "lipo", "-create", x86, arm, "-output", out); err != nil {
			return "", fmt.Errorf("agl: lipo: %w", err)
		}
		for _, arch := range []string{archX86, archARM} {
			if _, err := exec(ctx, "lipo", out, "-verify_arch", arch); err != nil {
				return "", fmt.Errorf("agl: missing %s slice: %w", arch, err)
			}
		}
		slog.Debug("agl: built", "sdk", sdk, "output", out)
		finish("Built the AGL stub library for x86_64 and arm64")
		return out, nil
	}
	return "", fmt.Errorf("agl: no installed macOS SDK built both slices: %w", last)
}

func compile(ctx context.Context, exec run.Exec, compiler, sdk, arch, minimum, src, out string) error {
	_, err := exec(ctx, compiler,
		"-arch", arch,
		"-dynamiclib",
		"-isysroot", sdk,
		"-mmacosx-version-min="+minimum,
		"-Wall",
		"-Wextra",
		"-Wpedantic",
		"-Werror",
		"-std="+cStandard,
		"-fstack-protector-strong",
		"-O2",
		"-D_FORTIFY_SOURCE=2",
		"-fPIE",
		"-o", out,
		"-install_name", installName,
		"-compatibility_version", compatVersion,
		"-current_version", currentVersion,
		src,
	)
	if err != nil {
		return fmt.Errorf("agl: compile %s: %w", arch, err)
	}
	return nil
}

func sdkCandidates(primary string) ([]string, error) {
	if primary == "" {
		return nil, errors.New("agl: SDK path is empty")
	}
	list := []string{primary}
	entries, err := os.ReadDir(filepath.Dir(primary))
	if err != nil {
		return nil, fmt.Errorf("agl: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "MacOSX") || !strings.HasSuffix(name, ".sdk") {
			continue
		}
		path := filepath.Join(filepath.Dir(primary), name)
		if path == primary {
			continue
		}
		list = append(list, path)
	}
	return list, nil
}

func Install(ctx context.Context, frameworkDir, binary string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("agl: %w", err)
	}
	if filepath.Base(frameworkDir) != frameworkName {
		return fmt.Errorf("agl: framework directory must be named %s", frameworkName)
	}
	version := filepath.Join(frameworkDir, "Versions", "A")
	resources := filepath.Join(version, "Resources")
	if err := os.MkdirAll(resources, dirMode); err != nil {
		return fmt.Errorf("agl: %w", err)
	}
	if err := tree.Copy(ctx, binary, filepath.Join(version, "AGL")); err != nil {
		return err
	}
	encoded, err := plist.Marshal(map[string]string{
		"CFBundleIdentifier": bundleID,
		"CFBundleName":       "AGL",
		"CFBundleVersion":    compatVersion,
	}, plist.XMLFormat)
	if err != nil {
		return fmt.Errorf("agl: %w", err)
	}
	if err := os.WriteFile(filepath.Join(resources, "Info.plist"), encoded, fileMode); err != nil {
		return fmt.Errorf("agl: %w", err)
	}
	links := [][2]string{
		{filepath.Join(frameworkDir, "Versions", "Current"), "A"},
		{filepath.Join(frameworkDir, "AGL"), "Versions/Current/AGL"},
		{filepath.Join(frameworkDir, "Resources"), "Versions/Current/Resources"},
	}
	for _, link := range links {
		if err := os.RemoveAll(link[0]); err != nil {
			return fmt.Errorf("agl: %w", err)
		}
		if err := os.Symlink(link[1], link[0]); err != nil {
			return fmt.Errorf("agl: symlink %s: %w", link[0], err)
		}
	}
	slog.Debug("agl: installed", "framework", frameworkDir, "binary", binary)
	return nil
}
