package cmd

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/openbunny/wormswmd/internal/apply"
	"github.com/openbunny/wormswmd/internal/check"
	"github.com/openbunny/wormswmd/internal/game"
	"github.com/openbunny/wormswmd/internal/qt"
)

func TestFixDoesNotFetch(t *testing.T) {
	cases := []struct {
		name           string
		scaffold       bool
		qt             string
		wantErrContain string
	}{
		{name: "missing app"},
		{name: "explicit archive", scaffold: true, qt: "missing.tar.gz", wantErrContain: "is absent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			app := filepath.Join(home, "missing.app")
			if tc.scaffold {
				var err error
				if app, err = game.Scaffold(t.Context(), t.TempDir()); err != nil {
					t.Fatalf("game.Scaffold: %v", err)
				}
			}
			args := []string{"fix", "--app", app, "--home", home, "--applications", t.TempDir()}
			if tc.qt != "" {
				args = append(args, "--qt", filepath.Join(home, tc.qt))
			}
			root := New()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(args)
			err := root.ExecuteContext(t.Context())
			var exit *ExitError
			if !errors.As(err, &exit) || exit.Code != 1 {
				t.Fatalf("ExecuteContext() error = %v, want exit 1", err)
			}
			if tc.wantErrContain != "" && (exit.Err == nil || !strings.Contains(exit.Err.Error(), tc.wantErrContain)) {
				t.Fatalf("ExecuteContext() error = %v, want %q", err, tc.wantErrContain)
			}
			if exit.Err != nil && strings.Contains(exit.Err.Error(), "wormswmd qt fetch") || strings.Contains(out.String(), "ready to open") || strings.Contains(out.String(), "will not open") {
				t.Fatalf("error = %v, output = %q", exit.Err, out.String())
			}
			cache := filepath.Join(home, "Library", "Caches", "wormswmd", qt.ArchiveName)
			if _, statErr := os.Lstat(cache); !errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("cache lstat error = %v", statErr)
			}
		})
	}
}

func TestEnsurePinnedQtWarnsOnlyOnBadArchive(t *testing.T) {
	cases := []struct {
		name     string
		contents []byte
		wantWarn bool
	}{
		{name: "absent"},
		{name: "corrupt", contents: []byte("not the pinned archive"), wantWarn: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), qt.ArchiveName)
			if tc.contents != nil {
				if err := os.WriteFile(path, tc.contents, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			prevFetch := fetchQt
			t.Cleanup(func() { fetchQt = prevFetch })
			fetched := false
			fetchQt = func(context.Context, string, string, string) error {
				fetched = true
				return nil
			}
			var buf bytes.Buffer
			prevLog := slog.Default()
			t.Cleanup(func() { slog.SetDefault(prevLog) })
			UseLogger(&buf, true)
			if err := ensurePinnedQt(t.Context(), path); err != nil {
				t.Fatal(err)
			}
			warned := strings.Contains(buf.String(), "failed verification")
			if !fetched || warned != tc.wantWarn {
				t.Fatalf("fetched = %v, warned = %v, log = %q", fetched, warned, buf.String())
			}
		})
	}
}

func TestWriteFixText(t *testing.T) {
	const app = "/Games/Worms W.M.D.app"
	cases := []struct {
		name   string
		result apply.Result
		report check.Report
		want   string
	}{
		{
			name:   "already",
			result: apply.Result{App: app, Already: true},
			report: check.Report{App: app, Ready: true},
			want:   "Already fixed: Worms W.M.D is ready to open.\n  App: " + app + "\n",
		},
		{
			name:   "applied",
			result: apply.Result{App: app, Backup: "/backup", Changes: []string{"Build an AGL stub."}},
			report: check.Report{App: app, Ready: true},
			want:   "Fixed: Worms W.M.D is ready to open.\n  App: " + app + "\n  Backup: /backup\n  Changes:\n    - Build an AGL stub.\n",
		},
		{
			name:   "not ready",
			result: apply.Result{App: app, Backup: "/backup", Warnings: []string{"low space"}},
			report: check.Report{App: app, Exit: check.ExitNotReady, Problems: []string{"QtCore is not version 5.15"}},
			want:   "Fixed, but not ready: Worms W.M.D will not open (exit 2).\n  App: " + app + "\n  Backup: /backup\n  Warnings:\n    - low space\n  Problems:\n    - QtCore is not version 5.15\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			command := &cobra.Command{}
			command.Flags().Bool("json", false, "")
			var out bytes.Buffer
			command.SetOut(&out)
			if err := writeFix(command, tc.result, tc.report); err != nil {
				t.Fatal(err)
			}
			if out.String() != tc.want {
				t.Fatalf("output = %q; want %q", out.String(), tc.want)
			}
		})
	}
}
