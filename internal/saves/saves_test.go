package saves

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openbunny/wormswmd/internal/game"
)

func fixtureHome(t *testing.T) (home, teamFile, steamDir string) {
	t.Helper()
	home = t.TempDir()
	teamFile = filepath.Join(home, "Library", "Application Support", "Team17", "save")
	steamDir = filepath.Join(home, "Library", "Application Support", "Steam", "userdata", "42", game.SteamAppID)
	mkdir(t, filepath.Dir(teamFile))
	mkdir(t, steamDir)
	write(t, teamFile, "original")
	write(t, filepath.Join(steamDir, "slot"), "steam-original")
	return home, teamFile, steamDir
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func TestBackupRestore(t *testing.T) {
	home, teamFile, steamDir := fixtureHome(t)
	steamFile := filepath.Join(steamDir, "slot")
	now := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	dir, err := Backup(t.Context(), home, now)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "Documents", backupRootName, "WormsWMD-SaveBackup-20200102-030405")
	if dir != want {
		t.Fatalf("backup = %s", dir)
	}
	names, err := List(t.Context(), home)
	if err != nil || len(names) != 1 || names[0] != dir {
		t.Fatalf("list = %#v, %v", names, err)
	}
	write(t, teamFile, "changed")
	write(t, steamFile, "steam-changed")
	if err := os.RemoveAll(steamDir); err != nil {
		t.Fatal(err)
	}
	if err := Restore(t.Context(), home, dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(teamFile)
	if err != nil || string(got) != "original" {
		t.Fatalf("team = %q, %v", got, err)
	}
	got, err = os.ReadFile(steamFile)
	if err != nil || string(got) != "steam-original" {
		t.Fatalf("steam = %q, %v", got, err)
	}
}

func TestBackupStampIsUTC(t *testing.T) {
	home := t.TempDir()
	team := filepath.Join(home, "Library", "Application Support", "Team17", "save")
	mkdir(t, filepath.Dir(team))
	write(t, team, "original")
	const utcMinus8 = -8 * time.Hour
	now := time.Date(2020, 1, 2, 3, 4, 5, 0, time.FixedZone("UTC-8", int(utcMinus8/time.Second)))
	dir, err := Backup(t.Context(), home, now)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "Documents", backupRootName, "WormsWMD-SaveBackup-20200102-110405")
	if dir != want {
		t.Fatalf("backup = %s, want %s", dir, want)
	}
}

func TestBackupAbsent(t *testing.T) {
	home := t.TempDir()
	_, err := Backup(t.Context(), home, time.Time{})
	if err == nil || !strings.Contains(err.Error(), absentSaves) {
		t.Fatalf("backup = %v", err)
	}
	names, err := List(t.Context(), home)
	if err != nil || len(names) != 0 {
		t.Fatalf("list = %#v, %v", names, err)
	}
	empty := filepath.Join(home, "Documents", backupRootName, "empty")
	mkdir(t, empty)
	if err := Restore(t.Context(), home, empty); err == nil || !strings.Contains(err.Error(), "has no saves") {
		t.Fatalf("restore empty = %v", err)
	}
}

func TestBackupTeam17Symlink(t *testing.T) {
	home := t.TempDir()
	parent := filepath.Join(home, "Library", "Application Support")
	mkdir(t, parent)
	target := filepath.Join(home, "linked-team")
	mkdir(t, target)
	link(t, target, filepath.Join(parent, teamDirName))
	_, err := Backup(t.Context(), home, time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC))
	if err == nil || strings.Contains(err.Error(), absentSaves) {
		t.Fatalf("backup = %v", err)
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("backup = %v", err)
	}
}

func TestRestoreSteamSymlinkLeavesUserdata(t *testing.T) {
	home, _, steamDir := fixtureHome(t)
	dir, err := Backup(t.Context(), home, time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, "outside.txt")
	write(t, outside, "kept")
	if err := os.RemoveAll(steamDir); err != nil {
		t.Fatal(err)
	}
	link(t, outside, steamDir)
	err = Restore(t.Context(), home, dir)
	if err == nil || !strings.Contains(err.Error(), "leaves userdata") {
		t.Fatalf("restore = %v", err)
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "kept" {
		t.Fatalf("outside = %q, %v", got, err)
	}
	info, err := os.Lstat(steamDir)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("destination = %v, %v", info, err)
	}
}

func TestBackupAndListRejectSymlinkRoot(t *testing.T) {
	home, _, _ := fixtureHome(t)
	docs := filepath.Join(home, "Documents")
	mkdir(t, docs)
	target := t.TempDir()
	link(t, target, filepath.Join(docs, backupRootName))
	_, err := Backup(t.Context(), home, time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC))
	if err == nil || strings.Contains(err.Error(), absentSaves) {
		t.Fatalf("backup = %v", err)
	}
	if _, err := List(t.Context(), home); err == nil {
		t.Fatal("symlink root listed")
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlink target = %#v", entries)
	}
}
