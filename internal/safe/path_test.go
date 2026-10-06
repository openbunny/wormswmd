package safe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanRel(t *testing.T) {
	got, err := CleanRel("Frameworks/QtCore.framework/QtCore")
	if err != nil || got != "Frameworks/QtCore.framework/QtCore" {
		t.Fatalf("CleanRel = %q, %v", got, err)
	}
	for _, bad := range []string{"", "/abs", "../x", "a/../../b", "a/\x00b", "ok/\n"} {
		if _, err := CleanRel(bad); err == nil {
			t.Fatalf("CleanRel(%q) succeeded", bad)
		}
	}
}

func TestInRoot(t *testing.T) {
	if err := InRoot("/app/Contents", "/app/Contents/MacOS/Worms W.M.D"); err != nil {
		t.Fatal(err)
	}
	if err := InRoot("/app/Contents", "/app/Contents"); err != nil {
		t.Fatal(err)
	}
	if err := InRoot("/app/Contents", "/tmp/other"); err == nil {
		t.Fatal("outside path accepted")
	}
}

func TestIntermediateEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	contents := filepath.Join(root, "Contents")
	if err := os.Mkdir(contents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := IntermediateEscape(root, filepath.Join(contents, "Frameworks")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(contents); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, contents); err != nil {
		t.Fatal(err)
	}
	err := IntermediateEscape(root, filepath.Join(contents, "Frameworks"))
	if err == nil || !strings.Contains(err.Error(), "leaves") {
		t.Fatalf("IntermediateEscape = %v", err)
	}
	plist := filepath.Join(root, "Info.plist")
	if err := os.Symlink(filepath.Join(outside, "plist"), plist); err != nil {
		t.Fatal(err)
	}
	if err := IntermediateEscape(root, plist); err != nil {
		t.Fatal(err)
	}
}

func FuzzCleanRel(f *testing.F) {
	f.Add("Frameworks/AGL")
	f.Add("../etc/passwd")
	f.Add("/tmp/x")
	f.Fuzz(func(t *testing.T, path string) {
		got, err := CleanRel(path)
		if err != nil {
			return
		}
		sep := string(filepath.Separator)
		escaped := got == ".." || strings.HasPrefix(got, ".."+sep)
		if escaped || strings.Contains(got, "\x00") || filepath.IsAbs(got) {
			t.Fatalf("CleanRel accepted %q as %q", path, got)
		}
	})
}
