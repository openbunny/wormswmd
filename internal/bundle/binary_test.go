package bundle

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	testDirMode  = 0o755
	testExecMode = 0o755
)

func TestBinaryPrefersVersion5Path(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "QtCore.framework")
	version := filepath.Join(dir, "Versions", "5")
	if err := os.MkdirAll(version, testDirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(version, "QtCore"), []byte("qt"), testExecMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("5", filepath.Join(dir, "Versions", "Current")); err != nil {
		t.Fatal(err)
	}
	got, err := Binary(dir, "QtCore")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "Versions", "5", "QtCore")
	if got != want {
		t.Fatalf("binary = %s", got)
	}
}

func TestBinaryMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Empty.framework")
	if err := os.MkdirAll(dir, testDirMode); err != nil {
		t.Fatal(err)
	}
	if _, err := Binary(dir, "Empty"); err == nil {
		t.Fatal("missing binary accepted")
	}
}
