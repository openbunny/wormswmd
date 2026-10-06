package agl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/openbunny/wormswmd/internal/run"
)

const (
	testDirMode  = 0o755
	testFileMode = 0o644
)

var aglExports = []string{
	"aglChoosePixelFormat",
	"aglDestroyPixelFormat",
	"aglNextPixelFormat",
	"aglDescribePixelFormat",
	"aglDevicesOfPixelFormat",
	"aglQueryRendererInfo",
	"aglDestroyRendererInfo",
	"aglNextRendererInfo",
	"aglDescribeRenderer",
	"aglCreateContext",
	"aglDestroyContext",
	"aglCopyContext",
	"aglUpdateContext",
	"aglSetCurrentContext",
	"aglGetCurrentContext",
	"aglSetDrawable",
	"aglSetFullScreen",
	"aglGetDrawable",
	"aglSetVirtualScreen",
	"aglGetVirtualScreen",
	"aglSetOffScreen",
	"aglGetOffScreen",
	"aglEnable",
	"aglDisable",
	"aglIsEnabled",
	"aglSetInteger",
	"aglGetInteger",
	"aglUseFont",
	"aglGetError",
	"aglErrorString",
	"aglSwapBuffers",
	"aglConfigure",
	"aglResetLibrary",
	"aglCreatePBuffer",
	"aglDestroyPBuffer",
	"aglDescribePBuffer",
	"aglTexImagePBuffer",
	"aglSetPBuffer",
	"aglGetPBuffer",
	"aglGetCGLContext",
	"aglGetCGLPixelFormat",
}

func TestExports(t *testing.T) {
	if !strings.Contains(source, "AGL_NO_VIRTUAL_SCREEN") {
		t.Fatal("source does not contain AGL_NO_VIRTUAL_SCREEN")
	}
	if !returnsNoVirtualScreen(source) {
		t.Fatal("aglGetVirtualScreen does not return AGL_NO_VIRTUAL_SCREEN")
	}
	if !strings.Contains(source, "return &agl_format") {
		t.Fatal("aglChoosePixelFormat does not return a pixel format")
	}
	if !strings.Contains(source, "return &agl_context") {
		t.Fatal("aglCreateContext does not return a context")
	}
	got := definedFunctions(source)
	want := make(map[string]struct{}, len(aglExports))
	for _, name := range aglExports {
		want[name] = struct{}{}
	}
	var missing, extra []string
	for name := range want {
		if _, ok := got[name]; !ok {
			missing = append(missing, name)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			extra = append(extra, name)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("missing %v extra %v", missing, extra)
	}
}

func TestBuildWithStubExec(t *testing.T) {
	sdk := filepath.Join(t.TempDir(), "SDKs", "MacOSX.sdk")
	if err := os.MkdirAll(sdk, testDirMode); err != nil {
		t.Fatal(err)
	}
	bin, err := Build(t.Context(), t.TempDir(), stubExec(sdk))
	if err != nil {
		t.Fatal(err)
	}
	if bin == "" {
		t.Fatal("build returned an empty path")
	}
	info, err := os.Stat(bin)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("build output mode %s", info.Mode())
	}
	framework := filepath.Join(t.TempDir(), "AGL.framework")
	if err := Install(t.Context(), framework, bin); err != nil {
		t.Fatal(err)
	}
	links := []struct {
		path string
		want string
	}{
		{path: filepath.Join(framework, "Versions", "Current"), want: "A"},
		{path: filepath.Join(framework, "AGL"), want: "Versions/Current/AGL"},
		{path: filepath.Join(framework, "Resources"), want: "Versions/Current/Resources"},
	}
	for _, link := range links {
		got, err := os.Readlink(link.path)
		if err != nil {
			t.Fatal(err)
		}
		if got != link.want {
			t.Fatalf("%s -> %s", link.path, got)
		}
	}
	plist, err := os.ReadFile(filepath.Join(framework, "Resources", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plist), "com.wormswmd.aglstub") {
		t.Fatalf("plist = %s", plist)
	}
	if err := Install(t.Context(), filepath.Join(t.TempDir(), "Other.framework"), bin); err == nil {
		t.Fatal("install accepted a directory not named AGL.framework")
	}
}

func TestBuildWithClang(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("GOOS is not darwin")
	}
	if _, err := exec.LookPath("xcrun"); err != nil {
		t.Skip("xcrun not found")
	}
	bin, err := Build(t.Context(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(t.Context(), "nm", "-gU", bin).CombinedOutput()
	if err != nil {
		t.Fatalf("nm: %v: %s", err, out)
	}
	seen := nmSymbols(out)
	var missing []string
	for _, name := range aglExports {
		if _, ok := seen[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing symbols %v", missing)
	}
}

func stubExec(sdk string) run.Exec {
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		switch {
		case name == "xcrun" && slices.Contains(args, "--find"):
			return []byte("/usr/bin/clang\n"), nil
		case name == "xcrun" && slices.Contains(args, "--show-sdk-path"):
			return []byte(sdk + "\n"), nil
		case strings.Contains(name, "clang"):
			return writeArgFile(args, "-o")
		case name == "lipo" && slices.Contains(args, "-create"):
			return writeArgFile(args, "-output")
		case name == "lipo" && slices.Contains(args, "-verify_arch"):
			return nil, nil
		default:
			return nil, fmt.Errorf("agl: unexpected command %s %q", name, args)
		}
	}
}

func writeArgFile(args []string, flag string) ([]byte, error) {
	out, err := argAfter(args, flag)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(out, []byte("agl"), testFileMode); err != nil {
		return nil, fmt.Errorf("agl: %w", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		return nil, fmt.Errorf("agl: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("agl: %s is not a regular file", out)
	}
	return nil, nil
}

func argAfter(args []string, flag string) (string, error) {
	for i, arg := range args {
		if arg != flag {
			continue
		}
		if i+1 >= len(args) {
			return "", fmt.Errorf("agl: %s has no value", flag)
		}
		return args[i+1], nil
	}
	return "", fmt.Errorf("agl: missing %s", flag)
}

func definedFunctions(src string) map[string]struct{} {
	got := make(map[string]struct{})
	for line := range strings.SplitSeq(src, "\n") {
		name, ok := definedFunction(line)
		if !ok {
			continue
		}
		got[name] = struct{}{}
	}
	return got
}

func definedFunction(line string) (string, bool) {
	if line == "" || strings.ContainsRune(" \t/#", rune(line[0])) || strings.HasPrefix(line, "static ") {
		return "", false
	}
	open := strings.IndexByte(line, '(')
	if open <= 0 {
		return "", false
	}
	if brace := strings.IndexByte(line, '{'); brace < open && !strings.HasSuffix(line, ",") {
		return "", false
	}
	fields := strings.Fields(line[:open])
	if len(fields) < 2 {
		return "", false
	}
	name := strings.TrimLeft(fields[len(fields)-1], "*")
	if !ident(name) {
		return "", false
	}
	return name, true
}

func ident(name string) bool {
	if name == "" {
		return false
	}
	for i := range len(name) {
		c := name[i]
		switch {
		case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		case i > 0 && c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return true
}

func returnsNoVirtualScreen(src string) bool {
	const sig = "aglGetVirtualScreen("
	i := strings.Index(src, sig)
	if i < 0 {
		return false
	}
	rest := src[i:]
	body, _, ok := strings.Cut(rest, "}")
	return ok && strings.Contains(body, "return AGL_NO_VIRTUAL_SCREEN")
}

func nmSymbols(out []byte) map[string]struct{} {
	seen := make(map[string]struct{})
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		sym := strings.TrimPrefix(fields[len(fields)-1], "_")
		seen[sym] = struct{}{}
	}
	return seen
}

func TestStubErrorCodes(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc not found")
	}
	bin := filepath.Join(t.TempDir(), "agl_errors")
	build := exec.CommandContext(t.Context(), cc, "-std=c11", "-Wall", "-Wextra", "-Wpedantic", "-Werror", "-Istub", "-o", bin, filepath.Join("stub", "agl_stub.c"), filepath.Join("testdata", "agl_errors.c"))
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cc: %v: %s", err, out)
	}
	failures := map[int]string{
		1: "aglConfigure did not return GL_FALSE with AGL_BAD_ENUM",
		2: "aglDestroyPBuffer did not return GL_FALSE with AGL_BAD_VALUE",
		3: "aglGetError did not clear the error",
	}
	var exit *exec.ExitError
	if err := exec.CommandContext(t.Context(), bin).Run(); errors.As(err, &exit) {
		t.Fatalf("agl_errors exit %d: %s", exit.ExitCode(), failures[exit.ExitCode()])
	} else if err != nil {
		t.Fatal(err)
	}
}
