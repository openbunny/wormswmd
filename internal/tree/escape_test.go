package tree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyRejectsSymlinkChainEscape(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "d", "e"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../..", filepath.Join(src, "d", "e", "a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("d/e/a/../..", filepath.Join(src, "f")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "copy")
	if err := Copy(t.Context(), src, dst); err == nil {
		t.Fatal("symlink chain escape copied")
	}
}

func TestCopyRefusesWriteThroughEscapingLink(t *testing.T) {
	src := t.TempDir()
	if err := os.Mkdir(filepath.Join(src, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "d", "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dst, "d")); err != nil {
		t.Fatal(err)
	}
	if err := Copy(t.Context(), src, dst); err == nil {
		t.Fatal("copy through a link leaving the destination succeeded")
	}
	if _, err := os.Stat(filepath.Join(outside, "file")); err == nil {
		t.Fatal("file written outside the destination")
	}
}
