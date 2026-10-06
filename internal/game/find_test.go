package game

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindExplicitAndAmbiguous(t *testing.T) {
	home := t.TempDir()
	apps := t.TempDir()
	first, err := Scaffold(t.Context(), filepath.Join(home, "Library", "Application Support", "Steam", "steamapps", "common", "WormsWMD"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(t.Context(), "", home, apps)
	if err != nil || got != first {
		t.Fatalf("Resolve = %q, %v", got, err)
	}
	secondDir := filepath.Join(apps, "nested")
	if _, err := Scaffold(t.Context(), secondDir); err != nil {
		t.Fatal(err)
	}
	_, err = Resolve(t.Context(), "", home, apps)
	var amb *AmbiguousError
	if !errors.As(err, &amb) || len(amb.Paths) != 2 {
		t.Fatalf("ambiguous = %#v", err)
	}
	if _, err := Resolve(t.Context(), first, home, apps); err != nil {
		t.Fatal(err)
	}
}

func TestLibraryPaths(t *testing.T) {
	data := "\"libraryfolders\"\n{\n\t\"0\"\n\t{\n\t\t\"path\"\t\t\"/Volumes/Games\"\n\t}\n}\n"
	got := LibraryPaths(data)
	if len(got) != 1 || got[0] != "/Volumes/Games" {
		t.Fatalf("paths = %#v", got)
	}
}

func FuzzLibraryPaths(f *testing.F) {
	f.Add("\"path\"\t\t\"/Volumes/Games\"\n")
	f.Add("\"path\" \"ok\x00no\"\n")
	f.Fuzz(func(t *testing.T, data string) {
		for _, path := range LibraryPaths(data) {
			if strings.Contains(path, "\x00") || strings.Contains(path, "\n") {
				t.Fatalf("LibraryPaths accepted %q", path)
			}
		}
	})
}

func TestSourceAndSaves(t *testing.T) {
	home := t.TempDir()
	app, err := Scaffold(t.Context(), home)
	if err != nil {
		t.Fatal(err)
	}
	source, err := Source(t.Context(), app)
	if err != nil || source != "unknown" {
		t.Fatalf("source = %s, %v", source, err)
	}
	write(t, filepath.Join(app, "Contents", "Frameworks", steamMarker), "x")
	source, err = Source(t.Context(), app)
	if err != nil || source != "steam" {
		t.Fatalf("source = %s, %v", source, err)
	}
	id := filepath.Join(home, "Library", "Application Support", "Steam", "userdata", "42", SteamAppID)
	mkdir(t, id)
	write(t, filepath.Join(id, "save"), "s")
	got, err := SteamSaveDirs(t.Context(), home)
	if err != nil || len(got) != 1 || got[0] != id {
		t.Fatalf("saves = %#v, %v", got, err)
	}
	outside := filepath.Join(home, "outside")
	mkdir(t, outside)
	link := filepath.Join(home, "Library", "Application Support", "Steam", "userdata", "7", SteamAppID)
	mkdir(t, filepath.Dir(link))
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := SteamSaveDirs(t.Context(), home); err == nil {
		t.Fatal("symlink save accepted")
	}
}

func TestResolveNotFound(t *testing.T) {
	_, err := Resolve(t.Context(), "", t.TempDir(), t.TempDir())
	if !errors.Is(err, ErrNotFound) || err.Error() != "game: Worms W.M.D.app not found; pass --app" {
		t.Fatalf("Resolve = %v", err)
	}
}

func TestValidClassifies(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.app")
	err := Valid(t.Context(), missing)
	if !errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrNotBundle) {
		t.Fatalf("missing = %v", err)
	}
	file := filepath.Join(t.TempDir(), "file.app")
	write(t, file, "x")
	err = Valid(t.Context(), file)
	if !errors.Is(err, ErrNotBundle) || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = Valid(ctx, file)
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrNotBundle) {
		t.Fatalf("canceled = %v", err)
	}
	deadline, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer stop()
	err = Valid(deadline, file)
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrNotBundle) {
		t.Fatalf("deadline = %v", err)
	}
	err = Valid(t.Context(), "Worms\nW.M.D.app")
	if !errors.Is(err, ErrNotBundle) {
		t.Fatalf("control = %v", err)
	}
}

func TestFindSkipsNotBundle(t *testing.T) {
	steamBundle := func(home string) string {
		return filepath.Join(home, "Library", "Application Support", "Steam", "steamapps", "common", "WormsWMD", bundleName)
	}
	cases := []struct {
		name       string
		homeSuffix string
		make       func(t *testing.T, home, apps string)
	}{
		{
			name: "file",
			make: func(t *testing.T, home, _ string) {
				path := steamBundle(home)
				mkdir(t, filepath.Dir(path))
				write(t, path, "x")
			},
		},
		{
			name: "symlink",
			make: func(t *testing.T, home, _ string) {
				path := steamBundle(home)
				target := filepath.Join(home, "target.app")
				mkdir(t, filepath.Dir(path))
				mkdir(t, target)
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "missing executable",
			make: func(t *testing.T, home, _ string) {
				path := filepath.Join(steamBundle(home), "Contents", "MacOS")
				mkdir(t, path)
			},
		},
		{
			name: "directory executable",
			make: func(t *testing.T, home, _ string) {
				path := filepath.Join(steamBundle(home), "Contents", "MacOS", execName)
				mkdir(t, path)
			},
		},
		{
			name:       "control character",
			homeSuffix: "bad\nhome",
			make: func(t *testing.T, home, _ string) {
				mkdir(t, home)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), tc.homeSuffix)
			apps := t.TempDir()
			tc.make(t, home, apps)
			found, err := Find(t.Context(), home, apps)
			if err != nil || len(found) != 0 {
				t.Fatalf("Find = %#v, %v", found, err)
			}
		})
	}
}

func TestFindMode000Candidate(t *testing.T) {
	home := t.TempDir()
	apps := t.TempDir()
	dir := filepath.Join(home, "Library", "Application Support", "Steam", "steamapps", "common", "WormsWMD", bundleName)
	mkdir(t, dir)
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Errorf("restore mode on %s: %v", dir, err)
		}
	})
	found, err := Find(t.Context(), home, apps)
	if os.Geteuid() == 0 {
		if err != nil || len(found) != 0 {
			t.Fatalf("Find = %#v, %v", found, err)
		}
		return
	}
	if err == nil || errors.Is(err, ErrNotBundle) || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Find = %#v, %v", found, err)
	}
}

func TestShallowWalkError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads mode 000 directories")
	}
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	mkdir(t, blocked)
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(blocked, 0o755); err != nil {
			t.Errorf("restore mode on %s: %v", blocked, err)
		}
	})
	if _, err := shallowBundles(t.Context(), root); err == nil {
		t.Fatal("unreadable directory was skipped")
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), testFileMode); err != nil {
		t.Fatal(err)
	}
}

const testFileMode = 0o644
