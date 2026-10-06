package tree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMeasureMatchesCopiedFiles(t *testing.T) {
	src := t.TempDir()
	if err := os.Mkdir(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"a": "12", "sub/b": "345"} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("a", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	want, err := Measure(t.Context(), src)
	if err != nil {
		t.Fatal(err)
	}
	var got Tally
	if err := CopyCounted(t.Context(), src, filepath.Join(t.TempDir(), "copy"), got.Add); err != nil {
		t.Fatal(err)
	}
	if got != want || got.Files != 2 || got.Bytes != 5 {
		t.Fatalf("copied %+v, measured %+v; want 2 files and 5 bytes", got, want)
	}
}

func TestTallyStringGroupsDigits(t *testing.T) {
	if got := (Tally{Files: 1234567, Bytes: 1}).String(); got != "1,234,567 files, 1 B" {
		t.Fatalf("got %q", got)
	}
	if got := (Tally{Files: 1}).String(); got != "1 file, 0 B" {
		t.Fatalf("got %q", got)
	}
}
