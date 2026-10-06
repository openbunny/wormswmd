package bundle

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var ErrNoBinary = errors.New("bundle: no binary")

func Binary(dir, name string) (string, error) {
	if name == "" {
		base := filepath.Base(dir)
		name = strings.TrimSuffix(base, ".framework")
	}
	candidates := []string{
		filepath.Join(dir, "Versions", "5", name),
		filepath.Join(dir, "Versions", "Current", name),
		filepath.Join(dir, "Versions", "A", name),
		filepath.Join(dir, name),
	}
	for _, candidate := range candidates {
		st, err := os.Stat(candidate)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("bundle: %s: %w", candidate, err)
		}
		if st.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("bundle: %s has no binary: %w", dir, ErrNoBinary)
}
