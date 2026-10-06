package backup

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

const (
	ufImmutable    = 0x2
	clearUserFlags = 0
)

func requireChflags(*testing.T) {}

func setImmutable(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Chflags(path, ufImmutable); err != nil {
		t.Fatal(err)
	}
}

func releaseImmutable(t *testing.T, root string) {
	t.Helper()
	t.Cleanup(func() {
		err := filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return syscall.Chflags(path, clearUserFlags)
		})
		if err != nil {
			t.Errorf("release immutable %s: %v", root, err)
		}
	})
}
