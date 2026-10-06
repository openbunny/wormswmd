package apply

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/openbunny/wormswmd/internal/game"
	"github.com/openbunny/wormswmd/internal/qt"
	"github.com/openbunny/wormswmd/internal/run"
	"howett.net/plist"
)

const (
	testDirMode  = 0o755
	testFileMode = 0o644
	testExecMode = 0o755
)

func TestEnsureQtOrder(t *testing.T) {
	t.Run("after the host check and before the backup", func(t *testing.T) {
		home := t.TempDir()
		app := scaffold(t, home)
		backup := filepath.Join(home, "backup")
		opt := baseOpt(app, home, "", backup)
		opt.CacheDir = t.TempDir()
		called := false
		opt.EnsureQt = func(_ context.Context, path string) error {
			called = true
			assertNoBackup(t, backup)
			want := filepath.Join(opt.CacheDir, "wormswmd", qt.ArchiveName)
			if path != want {
				t.Fatalf("path = %q, want %q", path, want)
			}
			return errors.New("ensure stopped")
		}
		opt.Exec = clangOnlyExec(t)
		_, err := Run(t.Context(), opt)
		if err == nil || !strings.Contains(err.Error(), "ensure stopped") || !called {
			t.Fatalf("Run error = %v, called = %v", err, called)
		}
		assertNoBackup(t, backup)
	})
	for _, tc := range []struct {
		name    string
		setup   func(t *testing.T, home string, opt *Options)
		wantErr string
		notErr  string
	}{
		{
			name: "explicit archive",
			setup: func(_ *testing.T, home string, opt *Options) {
				opt.QtArchive = filepath.Join(home, "missing.tar.gz")
			},
			wantErr: "is absent",
		},
		{
			name: "env archive",
			setup: func(_ *testing.T, home string, opt *Options) {
				opt.EnvQt = filepath.Join(home, "missing.tar.gz")
			},
			wantErr: "is absent",
			notErr:  "wormswmd qt fetch",
		},
		{
			name: "prefix",
			setup: func(t *testing.T, _ string, opt *Options) {
				opt.QtPrefix = qtPrefix(t)
				opt.Preview = true
			},
		},
		{
			name: "host check",
			setup: func(t *testing.T, _ string, opt *Options) {
				opt.MacOS = "15.0"
				opt.CacheDir = t.TempDir()
			},
			wantErr: "below major version",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			app := scaffold(t, home)
			opt := baseOpt(app, home, "", filepath.Join(home, "backup"))
			tc.setup(t, home, &opt)
			opt.EnsureQt = func(context.Context, string) error {
				t.Fatalf("EnsureQt called for %s", tc.name)
				return nil
			}
			opt.Exec = clangOnlyExec(t)
			_, err := Run(t.Context(), opt)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Run error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) || (tc.notErr != "" && strings.Contains(err.Error(), tc.notErr)) {
				t.Fatalf("Run error = %v", err)
			}
		})
	}
}

func TestFreeBytesMissingPath(t *testing.T) {
	_, err := freeBytes(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("freeBytes(missing) error = nil")
	}
}

func TestRun(t *testing.T) {
	t.Run("preview", func(t *testing.T) {
		home := t.TempDir()
		app := scaffold(t, home)
		prefix := qtPrefix(t)
		backup := filepath.Join(home, "backup")
		opt := baseOpt(app, home, prefix, backup)
		opt.Preview = true
		opt.Exec = func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("preview called exec")
			return nil, nil
		}
		result, err := Run(t.Context(), opt)
		if err != nil {
			t.Fatalf("Run error = %v", err)
		}
		if !result.Preview || result.Backup != "" {
			t.Fatalf("result = %+v", result)
		}
		joined := strings.Join(result.Changes, "\n")
		if !strings.Contains(joined, "Build the AGL stub library") || !strings.Contains(joined, "Replace the Qt libraries with Qt "+qt.Series) {
			t.Fatalf("changes = %v", result.Changes)
		}
		assertNoBackup(t, backup)
	})
	t.Run("old macos", func(t *testing.T) {
		home := t.TempDir()
		app := scaffold(t, home)
		backup := filepath.Join(home, "backup")
		opt := baseOpt(app, home, qtPrefix(t), backup)
		opt.MacOS = "15.0"
		opt.Exec = forbidExec(t)
		_, err := Run(t.Context(), opt)
		if err == nil {
			t.Fatal("Run error = nil")
		}
		assertNoBackup(t, backup)
	})
	t.Run("arm64 without rosetta", func(t *testing.T) {
		home := t.TempDir()
		app := scaffold(t, home)
		backup := filepath.Join(home, "backup")
		opt := baseOpt(app, home, qtPrefix(t), backup)
		opt.Arch = "arm64"
		opt.Exec = func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "arch" {
				return nil, errors.New("arch: cannot run x86_64")
			}
			t.Fatalf("exec %s %s", name, strings.Join(args, " "))
			return nil, nil
		}
		_, err := Run(t.Context(), opt)
		if err == nil || !strings.Contains(err.Error(), "--install-rosetta") {
			t.Fatalf("Run error = %v", err)
		}
		assertNoBackup(t, backup)
	})
	t.Run("missing archive", func(t *testing.T) {
		home := t.TempDir()
		app := scaffold(t, home)
		backup := filepath.Join(home, "backup")
		opt := baseOpt(app, home, "", backup)
		opt.QtArchive = filepath.Join(home, "missing.tar.gz")
		opt.Exec = clangOnlyExec(t)
		_, err := Run(t.Context(), opt)
		if err == nil || !strings.Contains(err.Error(), "is absent") || strings.Contains(err.Error(), "wormswmd qt fetch") {
			t.Fatalf("Run error = %v", err)
		}
		assertNoBackup(t, backup)
	})
	t.Run("compiler failure", func(t *testing.T) {
		home := t.TempDir()
		app := scaffold(t, home)
		backup := filepath.Join(home, "backup")
		opt := baseOpt(app, home, qtPrefix(t), backup)
		opt.Exec = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if name == "xcrun" {
				return stubTool(ctx, t, name, args, false)
			}
			if strings.HasSuffix(name, "clang") {
				return nil, errors.New("clang: compile failed")
			}
			t.Fatalf("exec %s", name)
			return nil, nil
		}
		_, err := Run(t.Context(), opt)
		if err == nil {
			t.Fatal("Run error = nil")
		}
		assertNoBackup(t, backup)
	})
	t.Run("codesign rollback", func(t *testing.T) {
		home := t.TempDir()
		app := scaffold(t, home)
		backup := filepath.Join(home, "backup")
		opt := baseOpt(app, home, qtPrefix(t), backup)
		opt.Exec = toolExec(t, true)
		_, err := Run(t.Context(), opt)
		if err == nil {
			t.Fatal("Run error = nil")
		}
		body, readErr := os.ReadFile(filepath.Join(app, "Contents", "Resources", "DataOSX", "SteamConfig.txt"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !strings.Contains(string(body), "http://www.team17.com") {
			t.Fatalf("SteamConfig = %s", body)
		}
		assertRestoredTrees(t, app, backup)
	})
	t.Run("codesign cancel rollback", func(t *testing.T) {
		home := t.TempDir()
		app := scaffold(t, home)
		backup := filepath.Join(home, "backup")
		opt := baseOpt(app, home, qtPrefix(t), backup)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		opt.Exec = func(runCtx context.Context, name string, args ...string) ([]byte, error) {
			if name == "codesign" {
				cancel()
				return nil, runCtx.Err()
			}
			return stubTool(runCtx, t, name, args, false)
		}
		_, err := Run(ctx, opt)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v", err)
		}
		const rel = "DataOSX/SteamConfig.txt"
		got, readErr := os.ReadFile(filepath.Join(app, "Contents", "Resources", rel))
		if readErr != nil {
			t.Fatal(readErr)
		}
		want, readErr := os.ReadFile(filepath.Join(backup, rel))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("SteamConfig = %q, backup = %q", got, want)
		}
		assertRestoredTrees(t, app, backup)
	})
	t.Run("contents symlink", func(t *testing.T) {
		home := t.TempDir()
		app := scaffold(t, home)
		outside := filepath.Join(t.TempDir(), "outside")
		contents := filepath.Join(app, "Contents")
		if err := os.Rename(contents, outside); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, contents); err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(outside, "Frameworks", "marker")
		if err := os.WriteFile(marker, []byte("keep"), testFileMode); err != nil {
			t.Fatal(err)
		}
		opt := baseOpt(app, home, qtPrefix(t), filepath.Join(home, "backup"))
		opt.Exec = forbidExec(t)
		_, err := Run(t.Context(), opt)
		if err == nil || !strings.Contains(err.Error(), "leaves") {
			t.Fatalf("Run error = %v", err)
		}
		body, readErr := os.ReadFile(marker)
		if readErr != nil || string(body) != "keep" {
			t.Fatalf("marker = %q, %v", body, readErr)
		}
	})
	t.Run("apply then already", func(t *testing.T) {
		home := t.TempDir()
		app := scaffold(t, home)
		prefix := qtPrefix(t)
		backup := filepath.Join(home, "backup")
		var calls []string
		opt := baseOpt(app, home, prefix, backup)
		opt.Exec = recordingExec(t, &calls, false)
		result, err := Run(t.Context(), opt)
		if err != nil {
			t.Fatalf("Run error = %v", err)
		}
		if result.Already || result.Backup == "" || result.Preview {
			t.Fatalf("result = %+v", result)
		}
		joined := strings.Join(calls, "\n")
		if strings.Contains(joined, "com.team17.wormswmd") {
			t.Fatalf("deleted lowercase domain:\n%s", joined)
		}
		if !strings.Contains(joined, "defaults delete com.team17.Worms W.M.D QtSystem_GameWindow.geometry") {
			t.Fatalf("window reset calls:\n%s", joined)
		}
		secondBackup := filepath.Join(home, "backup-2")
		again := baseOpt(app, home, prefix, secondBackup)
		again.Exec = toolExec(t, false)
		second, err := Run(t.Context(), again)
		if err != nil {
			t.Fatalf("second Run error = %v", err)
		}
		if !second.Already || second.Changes == nil || second.Warnings == nil {
			t.Fatalf("second = %+v", second)
		}
		assertNoBackup(t, secondBackup)
	})
}

func TestRunMissingClangStopsBeforeAnyChange(t *testing.T) {
	home := t.TempDir()
	app := scaffold(t, home)
	backup := filepath.Join(home, "backup")
	opt := baseOpt(app, home, "", backup)
	opt.CacheDir = t.TempDir()
	opt.EnsureQt = func(context.Context, string) error {
		t.Fatal("EnsureQt called before the clang check")
		return nil
	}
	opt.Exec = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "xcrun" {
			return nil, errors.New("xcrun: exit status 1")
		}
		t.Fatalf("exec %s %s", name, strings.Join(args, " "))
		return nil, nil
	}
	_, err := Run(t.Context(), opt)
	if err == nil || !strings.Contains(err.Error(), "install the Xcode Command Line Tools: xcode-select --install") {
		t.Fatalf("Run error = %v", err)
	}
	assertNoBackup(t, backup)
}

func TestRunPostMutateFailureNamesBackup(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool string
		args string
	}{
		{name: "quarantine", tool: "xattr", args: "-rd"},
		{name: "window", tool: "defaults", args: "delete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			app := scaffold(t, home)
			backup := filepath.Join(home, "backup")
			opt := baseOpt(app, home, qtPrefix(t), backup)
			opt.Exec = func(ctx context.Context, name string, args ...string) ([]byte, error) {
				if name == tc.tool && len(args) > 0 && args[0] == tc.args {
					return nil, errors.New(tc.tool + ": exit status 1")
				}
				return stubTool(ctx, t, name, args, false)
			}
			result, err := Run(t.Context(), opt)
			if err == nil {
				t.Fatal("Run error = nil")
			}
			if !strings.Contains(err.Error(), "the app was modified") || !strings.Contains(err.Error(), backup) || !strings.Contains(err.Error(), tc.tool+": exit status 1") {
				t.Fatalf("Run error = %v", err)
			}
			if result.Backup != backup {
				t.Fatalf("result.Backup = %q, want %q", result.Backup, backup)
			}
			if _, statErr := os.Stat(filepath.Join(app, "Contents", "Frameworks", "AGL.framework")); statErr != nil {
				t.Fatalf("the fix is not installed: %v", statErr)
			}
		})
	}
}

func assertNoBackup(t *testing.T, backup string) {
	t.Helper()
	if _, err := os.Lstat(backup); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("backup lstat error = %v", err)
	}
}

func assertRestoredTrees(t *testing.T, app, backup string) {
	t.Helper()
	pairs := []struct{ appRel, backupRel string }{
		{"Contents/Frameworks", "Frameworks"},
		{"Contents/PlugIns", "PlugIns"},
		{"Contents/MacOS", "MacOS"},
		{"Contents/Info.plist", "Info.plist"},
		{"Contents/Resources/DataOSX", "DataOSX"},
		{"Contents/Resources/CommonData", "CommonData"},
	}
	for _, pair := range pairs {
		sameTree(t, filepath.Join(app, pair.appRel), filepath.Join(backup, pair.backupRel))
	}
}

func sameTree(t *testing.T, left, right string) {
	t.Helper()
	got := treeEntries(t, left)
	want := treeEntries(t, right)
	for _, rel := range slices.Sorted(maps.Keys(got)) {
		if want[rel] != got[rel] {
			t.Fatalf("%s: app %q, backup %q", rel, got[rel], want[rel])
		}
	}
	for _, rel := range slices.Sorted(maps.Keys(want)) {
		if _, ok := got[rel]; !ok {
			t.Fatalf("%s missing from %s", rel, left)
		}
	}
}

func treeEntries(t *testing.T, root string) map[string]string {
	t.Helper()
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	if !info.IsDir() {
		out["."] = entryText(t, root, info)
		return out
	}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		st, err := d.Info()
		if err != nil {
			return err
		}
		out[rel] = entryText(t, path, st)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func entryText(t *testing.T, path string, info os.FileInfo) string {
	t.Helper()
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			t.Fatal(err)
		}
		return "link:" + target
	}
	if info.IsDir() {
		return "dir"
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return "file:" + string(body)
}

func TestReplacePathKeepsDestWhenCopyFails(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "QtCore.framework")
	if err := os.MkdirAll(dest, testDirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "marker"), []byte("old"), testFileMode); err != nil {
		t.Fatal(err)
	}
	err := replacePath(t.Context(), filepath.Join(dir, "missing"), dest)
	if err == nil {
		t.Fatal("replacePath error = nil")
	}
	got, readErr := os.ReadFile(filepath.Join(dest, "marker"))
	if readErr != nil || string(got) != "old" {
		t.Fatalf("marker = %q, %v", got, readErr)
	}
}

func TestAlreadyAppliedRejectsSystemAGL(t *testing.T) {
	home := t.TempDir()
	app := scaffold(t, home)
	agl := filepath.Join(app, "Contents", "Frameworks", "AGL.framework", "Versions", "A")
	if err := os.MkdirAll(agl, testDirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agl, "AGL"), []byte("agl"), testExecMode); err != nil {
		t.Fatal(err)
	}
	core := filepath.Join(app, "Contents", "Frameworks", "QtCore.framework", "Versions", "5", "Resources", "Info.plist")
	data, err := os.ReadFile(core)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(core, []byte(strings.ReplaceAll(string(data), "5.3.2", "5.15.19")), testFileMode); err != nil {
		t.Fatal(err)
	}
	qtgui := filepath.Join(app, "Contents", "Frameworks", "QtGui.framework", "Versions", "5")
	if err := os.MkdirAll(qtgui, testDirMode); err != nil {
		t.Fatal(err)
	}
	body := []byte("/System/Library/Frameworks/AGL.framework/Versions/A/AGL")
	if err := os.WriteFile(filepath.Join(qtgui, "QtGui"), body, testExecMode); err != nil {
		t.Fatal(err)
	}
	opt := baseOpt(app, home, qtPrefix(t), filepath.Join(home, "backup"))
	opt.Exec = forbidExec(t)
	ok, err := alreadyApplied(t.Context(), app, opt)
	if err != nil || ok {
		t.Fatalf("alreadyApplied = %v, %v", ok, err)
	}
}

func baseOpt(app, home, prefix, backup string) Options {
	return Options{
		App:       app,
		Home:      home,
		QtPrefix:  prefix,
		BackupDir: backup,
		MacOS:     "26.0",
		Arch:      "amd64",
		Now:       time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func scaffold(t *testing.T, home string) string {
	t.Helper()
	app, err := game.Scaffold(t.Context(), filepath.Join(home, "game"))
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func qtPrefix(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"QtCore", "QtDBus", "QtSvg"} {
		dir := filepath.Join(root, "Frameworks", name+".framework", "Versions", "5")
		if err := os.MkdirAll(filepath.Join(dir, "Resources"), testDirMode); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), testExecMode); err != nil {
			t.Fatal(err)
		}
		info, err := plist.Marshal(map[string]any{"CFBundleShortVersionString": "5.15.19"}, plist.XMLFormat)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Resources", "Info.plist"), info, testFileMode); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{
		filepath.Join(root, "Frameworks", "libpcre.dylib"),
		filepath.Join(root, "PlugIns", "platforms", "libqcocoa.dylib"),
		filepath.Join(root, "PlugIns", "imageformats", "libqsvg.dylib"),
	}
	for _, path := range files {
		if err := os.MkdirAll(filepath.Dir(path), testDirMode); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("dylib"), testFileMode); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func clangOnlyExec(t *testing.T) run.Exec {
	t.Helper()
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "xcrun" && slices.Equal(args, []string{"--find", "clang"}) {
			return []byte("/usr/bin/clang\n"), nil
		}
		t.Fatalf("exec %s %s", name, strings.Join(args, " "))
		return nil, nil
	}
}

func forbidExec(t *testing.T) run.Exec {
	t.Helper()
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		t.Fatalf("exec %s %s", name, strings.Join(args, " "))
		return nil, nil
	}
}

func toolExec(t *testing.T, verifyFail bool) run.Exec {
	t.Helper()
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return stubTool(ctx, t, name, args, verifyFail)
	}
}

func recordingExec(t *testing.T, calls *[]string, verifyFail bool) run.Exec {
	t.Helper()
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		*calls = append(*calls, name+" "+strings.Join(args, " "))
		return stubTool(ctx, t, name, args, verifyFail)
	}
}

func stubTool(ctx context.Context, t *testing.T, name string, args []string, verifyFail bool) ([]byte, error) {
	t.Helper()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch {
	case name == "xcrun" && slices.Contains(args, "--find"):
		return []byte("/usr/bin/clang\n"), nil
	case name == "xcrun" && slices.Contains(args, "--show-sdk-path"):
		sdk := filepath.Join(t.TempDir(), "MacOSX.sdk")
		if err := os.MkdirAll(sdk, testDirMode); err != nil {
			return nil, err
		}
		return []byte(sdk + "\n"), nil
	case strings.HasSuffix(name, "clang"):
		return nil, writeFlag(args, "-o")
	case name == "lipo" && slices.Contains(args, "-output"):
		return nil, writeFlag(args, "-output")
	case name == "lipo" || name == "otool" || name == "install_name_tool":
		return nil, nil
	case name == "codesign" && slices.Contains(args, "--verify") && verifyFail:
		return nil, errors.New("codesign: verify failed")
	case name == "codesign":
		return nil, nil
	case name == "xattr" && slices.Contains(args, "-p"):
		return []byte("No such xattr: com.apple.quarantine\n"), errors.New("xattr: exit status 1")
	case name == "defaults" && len(args) > 0 && args[0] == "read":
		return []byte("Domain not found\n"), errors.New("defaults: exit status 1")
	case name == "xattr" || name == "defaults":
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected exec %s %s", name, strings.Join(args, " "))
	}
}

func writeFlag(args []string, flag string) error {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return os.WriteFile(args[i+1], []byte("bin"), testExecMode)
		}
	}
	return fmt.Errorf("missing %s", flag)
}

func TestJoinRemoveKeepsBothErrors(t *testing.T) {
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.MkdirAll(filepath.Join(locked, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "sub", "file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(locked, "sub"), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(filepath.Join(locked, "sub"), 0o755); err != nil {
			t.Error(err)
		}
	})
	base := errors.New("rename failed")
	err := joinRemove(locked, base)
	if !errors.Is(err, base) || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("joinRemove = %v", err)
	}
}
