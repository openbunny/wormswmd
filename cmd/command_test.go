package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openbunny/wormswmd/internal/game"
	"github.com/openbunny/wormswmd/internal/qt"
)

func TestCheckExit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	scaffold, err := game.Scaffold(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("game.Scaffold: %v", err)
	}
	cases := []struct {
		name      string
		app       string
		code      int
		needsTool string
	}{
		{name: "missing", app: filepath.Join(t.TempDir(), "missing.app"), code: 1},
		{name: "scaffold", app: scaffold, code: 2, needsTool: "codesign"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.needsTool != "" {
				if _, err := exec.LookPath(tc.needsTool); err != nil {
					t.Skipf("check reports not ready only where %s exists, which is macOS", tc.needsTool)
				}
			}
			root := New()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs([]string{
				"check",
				"--app", tc.app,
				"--home", home,
				"--applications", t.TempDir(),
			})
			err := root.ExecuteContext(t.Context())
			var exit *ExitError
			if !errors.As(err, &exit) {
				t.Fatalf("ExecuteContext() error = %v, want *ExitError", err)
			}
			if exit.Code != tc.code {
				t.Fatalf("ExitError.Code = %d, want %d; output = %s", exit.Code, tc.code, out.String())
			}
		})
	}
}

func TestVersion(t *testing.T) {
	text := commandOutput(t, "version")
	line, ok := strings.CutSuffix(text, "\n")
	if !ok || line == "" || line == "0.0.0" || strings.Contains(line, "\n") {
		t.Fatalf("version = %q", text)
	}
}

func TestVersionJSON(t *testing.T) {
	plain := commandOutput(t, "version")
	text := commandOutput(t, "version", "--json")
	var got versionJSON
	if err := decodeOne(text, &got); err != nil {
		t.Fatalf("version JSON %q: %v", text, err)
	}
	if got.Version != strings.TrimSuffix(plain, "\n") {
		t.Fatalf("version = %q, plain = %q", got.Version, plain)
	}
}

func TestLaunchResolvesApp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	applications := filepath.Join(home, "Applications")
	app, err := game.Scaffold(t.Context(), applications)
	if err != nil {
		t.Fatalf("game.Scaffold: %v", err)
	}
	var calls [][]string
	prev := openExec
	t.Cleanup(func() { openExec = prev })
	openExec = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return nil, nil
	}
	root := New()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"launch", "--home", home, "--applications", applications})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("ExecuteContext() error = %v; output = %s", err, out.String())
	}
	if len(calls) != 1 || len(calls[0]) != 2 || calls[0][0] != "open" || calls[0][1] != app {
		t.Fatalf("open calls = %#v, want open %s", calls, app)
	}
}

func TestQtFetchPath(t *testing.T) {
	cache := filepath.Join("cache-dir", "user")
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{name: "empty", output: "", want: filepath.Join(cache, "wormswmd", qt.ArchiveName)},
		{name: "output", output: filepath.Join("chosen", "qt.tar.gz"), want: filepath.Join("chosen", "qt.tar.gz")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := qtFetchPath(tc.output, cache); got != tc.want {
				t.Fatalf("qtFetchPath(%q, %q) = %q, want %q", tc.output, cache, got, tc.want)
			}
		})
	}
}

func TestSupportPrintsArchivePath(t *testing.T) {
	home := t.TempDir()
	app, err := game.Scaffold(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("game.Scaffold: %v", err)
	}
	applications := t.TempDir()
	cases := []struct {
		name string
		json bool
	}{
		{name: "plain", json: false},
		{name: "json", json: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "report.tar")
			args := []string{"support", "--output", output, "--app", app, "--home", home, "--applications", applications}
			if tc.json {
				args = append(args, "--json")
			}
			text := commandOutput(t, args...)
			info, err := os.Stat(output)
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() == 0 {
				t.Fatal("support archive is empty")
			}
			if tc.json {
				var got supportJSON
				if err := decodeOne(text, &got); err != nil {
					t.Fatalf("support JSON %q: %v", text, err)
				}
				if got.Output != output {
					t.Fatalf("output = %q, want %q", got.Output, output)
				}
				return
			}
			if text != output+"\n" {
				t.Fatalf("support = %q, want %q", text, output)
			}
		})
	}
}

func commandOutput(t *testing.T, args ...string) string {
	t.Helper()
	root := New()
	var out, logs bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&logs)
	root.SetArgs(args)
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("ExecuteContext() error = %v; stdout = %s; stderr = %s", err, out.String(), logs.String())
	}
	return out.String()
}

func decodeOne(text string, v any) error {
	dec := json.NewDecoder(strings.NewReader(text))
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra any
	err := dec.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("extra JSON value")
}

func TestHelp(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		wants []string
	}{
		{name: "root", args: []string{"--help"}, wants: []string{"wormswmd"}},
		{name: "fix", args: []string{"fix", "--help"}, wants: []string{"--qt", "--qt-prefix", "--backup-dir", "--force"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := commandOutput(t, tc.args...)
			for _, want := range tc.wants {
				if !strings.Contains(text, want) {
					t.Fatalf("help = %q, want %s", text, want)
				}
			}
		})
	}
}
