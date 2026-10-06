package safe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinkEscape(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Versions", "5"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		link    string
		target  string
		wantErr bool
	}{
		{"sibling", "Versions/Current", "5", false},
		{"top level through current", "QtCore", "Versions/Current/QtCore", false},
		{"parent inside root", "Versions/5/x", "../5", false},
		{"leaves root", "Versions/x", "../../..", true},
		{"dotdot after name", "x", "Versions/../..", true},
		{"absolute", "x", "/tmp", true},
	}
	for _, c := range cases {
		err := LinkEscape(root, filepath.Join(root, c.link), c.target)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: LinkEscape = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}
