package macho

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPlan(t *testing.T) {
	const (
		bin      = "/Game/Worms W.M.D.app/Contents/MacOS/Worms W.M.D"
		contents = "/Game/Worms W.M.D.app/Contents"
		agl      = "@executable_path/../Frameworks/AGL.framework/Versions/A/AGL"
		pcre     = "@executable_path/../Frameworks/libpcre2-8.0.dylib"
	)
	index := Index{
		Contents: contents,
		Exec:     bin,
		Frameworks: map[string]string{
			"AGL":    agl,
			"QtCore": "@executable_path/../Frameworks/QtCore.framework/Versions/5/QtCore",
		},
		Dylibs: map[string]string{
			"libpcre2-8.0.dylib": pcre,
		},
		ExecRpaths: []string{"@executable_path/../Frameworks"},
	}
	inside := func(path string) bool {
		return path == contents || strings.HasPrefix(path, contents+"/")
	}
	files := map[string]bool{
		contents + "/Frameworks/libsteam_api.dylib": true,
		contents + "/Frameworks/libpcre2-8.0.dylib": true,
	}
	exists := func(path string) bool { return files[path] }
	img := Image{Path: bin, Rpaths: index.ExecRpaths}

	cases := []struct {
		name    string
		dep     Dep
		wantTo  string
		wantErr string
		warn    bool
	}{
		{name: "agl system path", dep: Dep{Path: "/System/Library/Frameworks/AGL.framework/Versions/A/AGL"}, wantTo: agl},
		{name: "system lib", dep: Dep{Path: "/usr/lib/libSystem.B.dylib"}},
		{name: "appkit stays", dep: Dep{Path: "/System/Library/Frameworks/AppKit.framework/Versions/C/AppKit"}},
		{name: "bundled executable path", dep: Dep{Path: "@executable_path/../Frameworks/libsteam_api.dylib"}},
		{name: "outside", dep: Dep{Path: "@executable_path/../../outside.dylib"}, wantErr: "outside"},
		{name: "missing strong", dep: Dep{Path: "@executable_path/../Frameworks/missing.dylib"}, wantErr: "missing"},
		{name: "missing weak", dep: Dep{Path: "@executable_path/../Frameworks/missing.dylib", Weak: true}, warn: true},
		{name: "dylib basename", dep: Dep{Path: "@rpath/libpcre2-8.0.dylib"}, wantTo: pcre},
		{name: "rpath inside", dep: Dep{Path: "@rpath/libsteam_api.dylib"}},
		{name: "rpath unresolved", dep: Dep{Path: "@rpath/libabsent.dylib"}, wantErr: "unresolved"},
		{name: "absolute", dep: Dep{Path: "/opt/local/lib/libz.dylib"}, wantErr: "unportable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img.Deps = []Dep{tc.dep}
			got, err := Plan(img, "", index, exists, inside)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.warn {
				if len(got.Warnings) != 1 {
					t.Fatalf("warnings = %#v", got.Warnings)
				}
				return
			}
			if len(got.Warnings) != 0 {
				t.Fatalf("warnings = %#v", got.Warnings)
			}
			if tc.wantTo == "" {
				if len(got.Edits) != 0 {
					t.Fatalf("edits = %#v", got.Edits)
				}
				return
			}
			if len(got.Edits) != 1 || got.Edits[0].From != tc.dep.Path || got.Edits[0].To != tc.wantTo {
				t.Fatalf("edits = %#v", got.Edits)
			}
		})
	}
}

func TestResolvedMissingAndDenied(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.dylib")
	got, err := resolved(missing)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(parent, "missing.dylib") {
		t.Fatalf("resolved = %q", got)
	}
	if os.Geteuid() == 0 {
		return
	}
	blocked := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(blocked, 0o755); err != nil {
			t.Errorf("restore mode on %s: %v", blocked, err)
		}
	})
	_, err = resolved(filepath.Join(blocked, "lib.dylib"))
	if err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("resolved = %v", err)
	}
}

func TestRewriteStatError(t *testing.T) {
	app := t.TempDir()
	secret := filepath.Join(app, "Contents", "secret")
	if err := os.MkdirAll(filepath.Join(app, "Contents", "Frameworks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(secret, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(secret, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(secret, 0o755); err != nil {
			t.Errorf("restore mode on %s: %v", secret, err)
		}
	})
	exec := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "otool" {
			t.Fatalf("exec %s %v", name, args)
		}
		if slices.Contains(args, "-L") {
			return []byte("bin:\n\t@executable_path/../secret/libx.dylib (compatibility version 1.0.0, current version 1.0.0)\n"), nil
		}
		return []byte("bin:\n"), nil
	}
	_, err := Rewrite(t.Context(), app, exec)
	if os.Geteuid() == 0 {
		if err == nil || !strings.Contains(err.Error(), "missing") {
			t.Fatalf("Rewrite = %v", err)
		}
		return
	}
	if err == nil || !errors.Is(err, os.ErrPermission) || !strings.HasPrefix(err.Error(), "macho:") || strings.Contains(err.Error(), "outside") || strings.Contains(err.Error(), "missing") {
		t.Fatalf("Rewrite = %v", err)
	}
}

func TestApplyPlanRequiresTheNewName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "QtGui")
	if err := os.WriteFile(path, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	system := "/System/Library/Frameworks/AGL.framework/Versions/A/AGL"
	line := func(dep string) []byte {
		return []byte("bin:\n\t" + dep + " (compatibility version 1.0.0, current version 1.0.0)\n")
	}
	plan := PlanResult{Edits: []Edit{{From: system, To: AGLID}}}
	cases := []struct {
		name     string
		otoolDep string
		wantErr  bool
	}{
		{name: "stuck", otoolDep: system, wantErr: true},
		{name: "fixed", otoolDep: AGLID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec := func(_ context.Context, name string, _ ...string) ([]byte, error) {
				switch name {
				case "install_name_tool":
					return nil, nil
				case "otool":
					return line(tc.otoolDep), nil
				}
				t.Fatalf("exec %s", name)
				return nil, nil
			}
			err := applyPlan(t.Context(), exec, path, plan)
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "was not changed") {
					t.Fatalf("applyPlan = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestParseDepsAndLoads(t *testing.T) {
	deps := ParseDeps("bin:\n\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1.0.0)\n\t/System/Library/Frameworks/AGL.framework/Versions/A/AGL (compatibility version 1.0.0, current version 1.0.0)\n")
	if len(deps) != 2 || deps[1] != "/System/Library/Frameworks/AGL.framework/Versions/A/AGL" {
		t.Fatalf("deps = %#v", deps)
	}
	weak, rpaths, id := ParseLoads("      cmd LC_ID_DYLIB\n     name libqdds.dylib (offset 24)\n      cmd LC_LOAD_WEAK_DYLIB\n     name /opt/missing.dylib (offset 24)\n      cmd LC_RPATH\n     path @executable_path/../Frameworks (offset 12)\n")
	if !weak["/opt/missing.dylib"] || len(rpaths) != 1 || rpaths[0] != "@executable_path/../Frameworks" || id != "libqdds.dylib" {
		t.Fatalf("weak = %#v rpaths = %#v id = %q", weak, rpaths, id)
	}
}

func TestLoadImageSkipsOwnID(t *testing.T) {
	exec := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "otool" {
			t.Fatalf("exec %s", name)
		}
		if slices.Contains(args, "-l") {
			return []byte("      cmd LC_ID_DYLIB\n     name libqdds.dylib (offset 24)\n      cmd LC_LOAD_DYLIB\n     name /usr/lib/libSystem.B.dylib (offset 24)\n"), nil
		}
		return []byte("libqdds.dylib:\n\tlibqdds.dylib (compatibility version 0.0.0, current version 0.0.0)\n\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1.0.0)\n"), nil
	}
	deps, _, err := loadImage(t.Context(), exec, "libqdds.dylib")
	if err != nil {
		t.Fatal(err)
	}
	if want := []Dep{{Path: "/usr/lib/libSystem.B.dylib"}}; !slices.Equal(deps, want) {
		t.Fatalf("deps = %#v; want %#v", deps, want)
	}
}

func FuzzParseDeps(f *testing.F) {
	f.Add("bin:\n\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1.0.0)\n")
	f.Add("")
	f.Fuzz(func(t *testing.T, out string) {
		for _, dep := range ParseDeps(out) {
			if strings.Contains(dep, "compatibility version") {
				t.Fatalf("dep %q", dep)
			}
		}
		ParseLoads(out)
	})
}

func TestRewriteThroughSymlinkedParent(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(link, "Worms W.M.D.app")
	for _, dir := range []string{"MacOS", "Frameworks"} {
		if err := os.MkdirAll(filepath.Join(app, "Contents", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{filepath.Join("MacOS", "Worms W.M.D"), filepath.Join("Frameworks", "libx.dylib")} {
		if err := os.WriteFile(filepath.Join(app, "Contents", file), []byte("bin"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	exec := func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch {
		case name == "otool" && slices.Contains(args, "-L"):
			return []byte("bin:\n\t@executable_path/../Frameworks/libx.dylib (compatibility version 1.0.0, current version 1.0.0)\n"), nil
		case name == "otool", name == "install_name_tool":
			return []byte("bin:\n"), nil
		}
		t.Fatalf("exec %s %v", name, args)
		return nil, nil
	}
	if _, err := Rewrite(t.Context(), app, exec); err != nil {
		t.Fatalf("Rewrite = %v", err)
	}
}
