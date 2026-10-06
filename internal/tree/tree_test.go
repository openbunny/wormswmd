package tree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyKeepsRelativeSymlinkInsideRoot(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "file"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../file", filepath.Join(src, "sub", "link")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "copy")
	if err := Copy(t.Context(), src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(filepath.Join(dst, "sub", "link"))
	if err != nil || got != "../file" {
		t.Fatalf("link = %q, %v", got, err)
	}
	body, err := os.ReadFile(filepath.Join(dst, "file"))
	if err != nil || string(body) != "body" {
		t.Fatalf("file = %q, %v", body, err)
	}
}

func TestCopyRejectsEscapingSymlink(t *testing.T) {
	src := t.TempDir()
	if err := os.Mkdir(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../outside", filepath.Join(src, "sub", "link")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "copy")
	if err := Copy(t.Context(), src, dst); err == nil {
		t.Fatal("escaping symlink copied")
	}
}

func TestCopyRejectsAbsoluteSymlink(t *testing.T) {
	src := t.TempDir()
	if err := os.Symlink("/tmp/outside", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "copy")
	if err := Copy(t.Context(), src, dst); err == nil {
		t.Fatal("absolute symlink copied")
	}
}

func TestCopyMakesReadOnlyFileOwnerWritable(t *testing.T) {
	src := filepath.Join(t.TempDir(), "Info.plist")
	if err := os.WriteFile(src, []byte("plist"), 0o444); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "Info.plist")
	if err := Copy(t.Context(), src, dst); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("mode = %o; want 644", got)
	}
}
