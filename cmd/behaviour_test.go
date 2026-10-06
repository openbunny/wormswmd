package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openbunny/wormswmd/internal/check"
	"github.com/openbunny/wormswmd/internal/game"
	"github.com/openbunny/wormswmd/internal/qt"
)

func TestCheckHeadline(t *testing.T) {
	cases := []struct {
		name   string
		report check.Report
		want   string
	}{
		{name: "ready", report: check.Report{Ready: true}, want: "Ready: Worms W.M.D can open."},
		{name: "missing", report: check.Report{Exit: check.ExitMissing}, want: "Not found: no Worms W.M.D app was located (exit 1)."},
		{name: "ambiguous", report: check.Report{Exit: check.ExitMissing, Ambiguous: true}, want: "Not found: several Worms W.M.D apps were located; pass --app (exit 1)."},
		{name: "not ready", report: check.Report{Exit: check.ExitNotReady}, want: "Not ready: Worms W.M.D will not open (exit 2)."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkHeadline(tc.report); got != tc.want {
				t.Fatalf("checkHeadline = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckMissingPrintsNotFound(t *testing.T) {
	home := t.TempDir()
	root := New()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"check", "--home", home, "--applications", t.TempDir()})
	err := root.ExecuteContext(t.Context())
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("ExecuteContext() error = %v, want exit 1", err)
	}
	if !strings.Contains(out.String(), "Not found: no Worms W.M.D app was located (exit 1).") || strings.Contains(out.String(), "will not open") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRestoredLine(t *testing.T) {
	if got, want := restoredLine("/Games/Worms W.M.D.app", "/backup"), "Restored /Games/Worms W.M.D.app from the backup /backup."; got != want {
		t.Fatalf("restoredLine = %q, want %q", got, want)
	}
	if got := restoredLine("", "/backup"); !strings.Contains(got, "/backup") {
		t.Fatalf("restoredLine = %q", got)
	}
}

func TestSavesRestorePrintsPriorBackup(t *testing.T) {
	home := t.TempDir()
	team := filepath.Join(home, "Library", "Application Support", "Team17")
	if err := os.MkdirAll(team, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(team, "slot"), []byte("current"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(home, "Documents", "WormsWMD-SaveBackups", "old")
	if err := os.MkdirAll(filepath.Join(old, "Team17"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "Team17", "slot"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	text := commandOutput(t, "saves", "restore", "--dir", old, "--home", home)
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	root := filepath.Join(home, "Documents", "WormsWMD-SaveBackups")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "Saved the current saves to "+root) || lines[1] != "Restored the saves from "+old {
		t.Fatalf("output = %q", text)
	}
	prior := strings.TrimPrefix(lines[0], "Saved the current saves to ")
	got, err := os.ReadFile(filepath.Join(prior, "Team17", "slot"))
	if err != nil || string(got) != "current" {
		t.Fatalf("prior backup = %q, %v", got, err)
	}
}

func TestQtSHA256NeedsArchive(t *testing.T) {
	t.Setenv("WORMSWMD_QT", "")
	home := t.TempDir()
	app, err := game.Scaffold(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fix", "apply", "preview"} {
		t.Run(name, func(t *testing.T) {
			root := New()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs([]string{name, "--app", app, "--home", home, "--applications", t.TempDir(), "--qt-sha256", strings.Repeat("0", 64)})
			err := root.ExecuteContext(t.Context())
			var exit *ExitError
			if !errors.As(err, &exit) || exit.Code != 1 || !strings.Contains(err.Error(), "--qt-sha256 applies to an archive given by --qt or WORMSWMD_QT") {
				t.Fatalf("ExecuteContext() error = %v", err)
			}
		})
	}
}

func TestDownloadErrorNamesURLPinAndRemedy(t *testing.T) {
	cause := errors.New("dial tcp: no route")
	err := downloadError(cause)
	if !errors.Is(err, cause) {
		t.Fatalf("downloadError = %v", err)
	}
	for _, want := range []string{qt.ArchiveURL, qt.PinSHA256, "download the archive another way and pass --qt PATH"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("downloadError = %q, want %q", err, want)
		}
	}
}

func TestHelpIsAccurate(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		wants  []string
		unwant []string
	}{
		{name: "preview", args: []string{"preview", "--help"}, wants: []string{"Nothing is written"}, unwant: []string{"Apply when", "--backup-dir", "--install-rosetta"}},
		{name: "apply", args: []string{"apply", "--help"}, wants: []string{"Write the changes when", "Directory that receives the app backup"}},
		{name: "root", args: []string{"--help"}, wants: []string{"wormswmd fix", "macOS 26 or later", "searched for the game"}},
		{name: "support", args: []string{"support", "--help"}, wants: []string{"tar archive"}},
		{name: "restore", args: []string{"restore", "--help"}, wants: []string{"--backup is required"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := commandOutput(t, tc.args...)
			for _, want := range tc.wants {
				if !strings.Contains(text, want) {
					t.Fatalf("help = %q, want %q", text, want)
				}
			}
			for _, unwant := range tc.unwant {
				if strings.Contains(text, unwant) {
					t.Fatalf("help = %q, must not contain %q", text, unwant)
				}
			}
		})
	}
}
