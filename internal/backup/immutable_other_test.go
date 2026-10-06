//go:build !darwin

package backup

import "testing"

func requireChflags(t *testing.T) {
	t.Helper()
	t.Skip("immutable file flags need BSD chflags, which this OS lacks")
}

func setImmutable(t *testing.T, path string) {
	t.Helper()
	t.Fatalf("setImmutable %s runs only after requireChflags", path)
}

func releaseImmutable(*testing.T, string) {}
