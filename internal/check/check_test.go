package check

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openbunny/wormswmd/internal/game"
	"howett.net/plist"
)

const (
	testDirMode  = 0o755
	testFileMode = 0o644
	testExecMode = 0o755
)

func TestMajorAndStatus(t *testing.T) {
	majorCases := []struct {
		in    string
		want  int
		fails bool
	}{
		{in: "26.0", want: 26},
		{in: "15.6.1", want: 15},
		{in: "26", want: 26},
		{in: "", fails: true},
		{in: "abc", fails: true},
		{in: "v26.0", fails: true},
	}
	for _, tc := range majorCases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := Major(tc.in)
			if tc.fails {
				if err == nil {
					t.Fatalf("Major(%q) error = nil", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("Major(%q) error = %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("Major(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
	if Status(ExitReady) != "ready" || Status(ExitMissing) != "missing" || Status(ExitNotReady) != "not-ready" {
		t.Fatalf("Status constants = %s %s %s", Status(ExitReady), Status(ExitMissing), Status(ExitNotReady))
	}
	if Status(9) != "exit status 9" {
		t.Fatalf("Status(9) = %q", Status(9))
	}
}

func TestEvaluate(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		app := filepath.Join(t.TempDir(), "missing.app")
		report, err := Evaluate(t.Context(), app, t.TempDir(), t.TempDir(), hostProbes("26.0", "amd64"))
		if err != nil {
			t.Fatalf("Evaluate error = %v", err)
		}
		if report.Exit != ExitMissing || !strings.Contains(report.Problems[0], "not found") {
			t.Fatalf("report = %+v", report)
		}
		if report.Problems == nil || report.Notes == nil {
			t.Fatal("Problems or Notes is nil")
		}
	})
	for _, tc := range []struct {
		name        string
		app         func(*testing.T) string
		probes      Probes
		wantExit    int
		wantProblem string
		wantNote    string
	}{
		{
			name: "not a bundle",
			app: func(t *testing.T) string {
				app := filepath.Join(t.TempDir(), "Worms W.M.D.app")
				if err := os.WriteFile(app, []byte("nope"), testFileMode); err != nil {
					t.Fatal(err)
				}
				return app
			},
			probes:   hostProbes("26.0", "amd64"),
			wantExit: ExitNotReady,
		},
		{name: "scaffold", app: scaffoldApp, probes: hostProbes("26.0", "amd64"), wantExit: ExitNotReady},
		{name: "system agl", app: systemAGLApp, probes: hostProbes("26.0", "amd64"), wantExit: ExitNotReady, wantProblem: "system AGL"},
		{name: "ready", app: readyApp, probes: hostProbes("26.0", "amd64"), wantExit: ExitReady},
		{name: "old macos", app: readyApp, probes: hostProbes("15.0", "amd64"), wantExit: ExitReady, wantNote: "below major version"},
		{
			name: "compiler note",
			app:  readyApp,
			probes: func() Probes {
				probes := hostProbes("26.0", "amd64")
				probes.Compiler = func(context.Context) error { return errors.New("clang is absent") }
				return probes
			}(),
			wantExit: ExitReady,
			wantNote: "clang: clang is absent",
		},
		{
			name: "rosetta absent",
			app:  readyApp,
			probes: func() Probes {
				probes := hostProbes("26.0", "arm64")
				probes.Rosetta = func(context.Context) (bool, error) { return false, nil }
				return probes
			}(),
			wantExit:    ExitNotReady,
			wantProblem: "Rosetta is absent",
		},
		{
			name: "unsigned",
			app:  readyApp,
			probes: func() Probes {
				probes := hostProbes("26.0", "amd64")
				probes.Signature = func(context.Context, string) (bool, error) { return false, nil }
				return probes
			}(),
			wantExit:    ExitNotReady,
			wantProblem: "code signature does not verify",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := Evaluate(t.Context(), tc.app(t), t.TempDir(), t.TempDir(), tc.probes)
			if err != nil {
				t.Fatalf("Evaluate error = %v", err)
			}
			if report.Exit != tc.wantExit || report.Ready != (tc.wantExit == ExitReady) ||
				!strings.Contains(strings.Join(report.Problems, "\n"), tc.wantProblem) ||
				!strings.Contains(strings.Join(report.Notes, "\n"), tc.wantNote) {
				t.Fatalf("report = %+v", report)
			}
		})
	}
	t.Run("compiler deadline", func(t *testing.T) {
		app := readyApp(t)
		probes := hostProbes("26.0", "amd64")
		probes.Compiler = func(context.Context) error {
			return fmt.Errorf("run: xcrun: %w", context.DeadlineExceeded)
		}
		report, err := Evaluate(t.Context(), app, t.TempDir(), t.TempDir(), probes)
		if !errors.Is(err, context.DeadlineExceeded) || len(report.Notes) != 0 {
			t.Fatalf("report = %+v, %v", report, err)
		}
	})
	t.Run("rosetta error", func(t *testing.T) {
		app := readyApp(t)
		probes := hostProbes("26.0", "arm64")
		probes.Rosetta = func(context.Context) (bool, error) { return false, errors.New("cannot tell") }
		_, err := Evaluate(t.Context(), app, t.TempDir(), t.TempDir(), probes)
		if err == nil {
			t.Fatal("Evaluate error = nil")
		}
	})
	t.Run("search miss", func(t *testing.T) {
		report, err := Evaluate(t.Context(), "", t.TempDir(), t.TempDir(), hostProbes("26.0", "amd64"))
		if err != nil {
			t.Fatalf("Evaluate error = %v", err)
		}
		if report.Exit != ExitMissing || len(report.Problems) != 1 || !strings.Contains(report.Problems[0], "not found") {
			t.Fatalf("report = %+v", report)
		}
	})
	t.Run("ambiguous", func(t *testing.T) {
		home := t.TempDir()
		apps := t.TempDir()
		if _, err := game.Scaffold(t.Context(), filepath.Join(home, "Applications")); err != nil {
			t.Fatal(err)
		}
		if _, err := game.Scaffold(t.Context(), apps); err != nil {
			t.Fatal(err)
		}
		report, err := Evaluate(t.Context(), "", home, apps, hostProbes("26.0", "amd64"))
		if err != nil {
			t.Fatalf("Evaluate error = %v", err)
		}
		if report.Exit != ExitMissing || !strings.Contains(strings.Join(report.Problems, "\n"), "pass --app") {
			t.Fatalf("report = %+v", report)
		}
	})
}

func systemAGLApp(t *testing.T) string {
	t.Helper()
	app := readyApp(t)
	qtgui := filepath.Join(app, "Contents", "Frameworks", "QtGui.framework", "Versions", "5")
	if err := os.MkdirAll(qtgui, testDirMode); err != nil {
		t.Fatal(err)
	}
	body := []byte("/System/Library/Frameworks/AGL.framework/Versions/A/AGL")
	if err := os.WriteFile(filepath.Join(qtgui, "QtGui"), body, testExecMode); err != nil {
		t.Fatal(err)
	}
	return app
}

func hostProbes(macOS, arch string) Probes {
	return Probes{
		MacOS: macOS,
		Arch:  arch,
		Compiler: func(context.Context) error {
			return nil
		},
		Signature: func(context.Context, string) (bool, error) {
			return true, nil
		},
		Quarantine: func(context.Context, string) (bool, error) {
			return false, nil
		},
		Window: func(context.Context) (bool, error) {
			return false, nil
		},
	}
}

func scaffoldApp(t *testing.T) string {
	t.Helper()
	app, err := game.Scaffold(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func readyApp(t *testing.T) string {
	t.Helper()
	app := scaffoldApp(t)
	agl := filepath.Join(app, "Contents", "Frameworks", "AGL.framework", "Versions", "A")
	if err := os.MkdirAll(agl, testDirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agl, "AGL"), []byte("agl"), testExecMode); err != nil {
		t.Fatal(err)
	}
	info, err := plist.Marshal(map[string]any{
		"CFBundleIdentifier":                   "com.team17.wormswmd",
		"NSHighResolutionCapable":              true,
		"NSSupportsAutomaticGraphicsSwitching": true,
		"LSMinimumSystemVersion":               "10.13",
	}, plist.XMLFormat)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), info, testFileMode); err != nil {
		t.Fatal(err)
	}
	qtPlist := filepath.Join(app, "Contents", "Frameworks", "QtCore.framework", "Versions", "5", "Resources", "Info.plist")
	data, err := os.ReadFile(qtPlist)
	if err != nil {
		t.Fatal(err)
	}
	next := strings.ReplaceAll(string(data), "5.3.2", "5.15.19")
	if next == string(data) {
		t.Fatal("QtCore plist has no 5.3.2")
	}
	if err := os.WriteFile(qtPlist, []byte(next), testFileMode); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestResolveCancelled(t *testing.T) {
	app := scaffoldApp(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, report, err := resolve(ctx, app, t.TempDir(), t.TempDir())
	if !errors.Is(err, context.Canceled) || report.Exit != ExitReady {
		t.Fatalf("resolve = %+v, %v", report, err)
	}
}

func TestRosettaCommandExit(t *testing.T) {
	app := readyApp(t)
	absent := hostProbes("26.0", "arm64")
	absent.Exec = func(ctx context.Context, name string, _ ...string) ([]byte, error) {
		if name != "arch" {
			t.Fatalf("exec %s", name)
		}
		cmd := exec.CommandContext(ctx, "false")
		return nil, fmt.Errorf("run: %s: %w", name, cmd.Run())
	}
	report, err := Evaluate(t.Context(), app, t.TempDir(), t.TempDir(), absent)
	if err != nil || report.Exit != ExitNotReady || !strings.Contains(strings.Join(report.Problems, "\n"), "Rosetta is absent") {
		t.Fatalf("report = %+v, %v", report, err)
	}

	missing := hostProbes("26.0", "arm64")
	missing.Exec = func(_ context.Context, name string, _ ...string) ([]byte, error) {
		return nil, fmt.Errorf("run: %s: %w", name, &exec.Error{Name: name, Err: exec.ErrNotFound})
	}
	_, err = Evaluate(t.Context(), app, t.TempDir(), t.TempDir(), missing)
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("Evaluate error = %v", err)
	}
}

func TestQtBinaryPlist(t *testing.T) {
	app := readyApp(t)
	path := filepath.Join(app, "Contents", "Frameworks", "QtCore.framework", "Versions", "5", "Resources", "Info.plist")
	bin, err := plist.Marshal(map[string]any{
		"CFBundleShortVersionString": "5.15.19",
	}, plist.BinaryFormat)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bin, testFileMode); err != nil {
		t.Fatal(err)
	}
	probes := hostProbes("26.0", "amd64")
	probes.Exec = func(_ context.Context, name string, _ ...string) ([]byte, error) {
		t.Errorf("exec %s", name)
		return nil, fmt.Errorf("exec %s", name)
	}
	report, err := Evaluate(t.Context(), app, t.TempDir(), t.TempDir(), probes)
	if err != nil || report.Exit != ExitReady || !report.Ready {
		t.Fatalf("report = %+v, %v", report, err)
	}
}

func TestSignatureAndQuarantineErrors(t *testing.T) {
	exitErr := exec.CommandContext(t.Context(), "false").Run()
	if _, ok := errors.AsType[*exec.ExitError](exitErr); !ok {
		t.Fatalf("false = %v", exitErr)
	}
	canceled := fmt.Errorf("run: tool: %w", context.Canceled)
	deadline := fmt.Errorf("run: tool: %w", context.DeadlineExceeded)
	absent := fmt.Errorf("run: tool: %w", exec.ErrNotFound)
	unsigned := fmt.Errorf("run: codesign: %w", exitErr)
	other := errors.New("codesign: boom")
	app := "Worms W.M.D.app"

	signatureCases := []struct {
		name    string
		err     error
		wantErr error
		ok      bool
	}{
		{name: "canceled", err: canceled, wantErr: context.Canceled},
		{name: "deadline", err: deadline, wantErr: context.DeadlineExceeded},
		{name: "absent", err: absent, wantErr: exec.ErrNotFound},
		{name: "unsigned", err: unsigned},
		{name: "other", err: other, wantErr: other},
		{name: "signed", ok: true},
	}
	for _, tc := range signatureCases {
		t.Run("signature "+tc.name, func(t *testing.T) {
			probes := Probes{Exec: func(context.Context, string, ...string) ([]byte, error) {
				if tc.ok {
					return nil, nil
				}
				return nil, tc.err
			}}
			got, err := signature(t.Context(), app, probes)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || got {
					t.Fatalf("signature = %v, %v", got, err)
				}
				return
			}
			if err != nil || got != tc.ok {
				t.Fatalf("signature = %v, %v", got, err)
			}
		})
	}

	xattrAbsent := []byte("xattr: /app: No such xattr: com.apple.quarantine\n")
	for _, tc := range []struct {
		name    string
		out     []byte
		err     error
		wantGot bool
		wantErr error
	}{
		{name: "absent xattr", out: xattrAbsent, err: fmt.Errorf("run: xattr: %w: %s", exitErr, xattrAbsent)},
		{name: "text only in error", err: fmt.Errorf("run: xattr: %w: No such xattr", exitErr), wantErr: exitErr},
		{name: "canceled before xattr text", out: []byte("No such xattr"), err: canceled, wantErr: context.Canceled},
		{name: "present", out: []byte("0083;00000000"), wantGot: true},
		{name: "deadline", err: deadline, wantErr: deadline},
		{name: "absent", err: absent, wantErr: absent},
	} {
		t.Run("quarantine "+tc.name, func(t *testing.T) {
			probes := Probes{Exec: func(context.Context, string, ...string) ([]byte, error) {
				return tc.out, tc.err
			}}
			got, err := quarantine(t.Context(), app, probes)
			if got != tc.wantGot || (tc.wantErr == nil) != (err == nil) || (tc.wantErr != nil && !errors.Is(err, tc.wantErr)) {
				t.Fatalf("quarantine = %v, %v", got, err)
			}
		})
	}
}

func TestSignaledExit(t *testing.T) {
	waitErr := exec.CommandContext(t.Context(), "/bin/sh", "-c", "kill -9 $$").Run()
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) || exitErr.ExitCode() != exitCodeNotExited {
		t.Fatalf("wait = %v", waitErr)
	}
	probes := Probes{Exec: func(context.Context, string, ...string) ([]byte, error) {
		return nil, fmt.Errorf("run: tool: %w", waitErr)
	}}
	ok, err := rosetta(t.Context(), probes)
	var got *exec.ExitError
	if ok || !errors.As(err, &got) || got.ExitCode() != exitCodeNotExited {
		t.Fatalf("rosetta = %v, %v", ok, err)
	}
	ok, err = signature(t.Context(), "Worms W.M.D.app", probes)
	got = nil
	if ok || !errors.As(err, &got) || got.ExitCode() != exitCodeNotExited {
		t.Fatalf("signature = %v, %v", ok, err)
	}
}

func TestFileContains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(path, []byte("prefix /System/Library/Frameworks/AGL.framework/ suffix"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		cancel  bool
		needle  string
		want    bool
		wantErr error
	}{
		{name: "hit", needle: systemAGLPrefix, want: true},
		{name: "miss", needle: "absent"},
		{name: "canceled", cancel: true, needle: systemAGLPrefix, wantErr: context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			if tc.cancel {
				cancel()
			}
			defer cancel()
			got, err := fileContains(ctx, path, tc.needle)
			if got != tc.want || !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) {
				t.Fatalf("fileContains = %v, %v", got, err)
			}
		})
	}
}

var errClose = errors.New("close failed")

type closeFailFile struct{ *os.File }

func (f closeFailFile) Close() error { return errors.Join(f.File.Close(), errClose) }

func TestFileContainsReturnsCloseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(path, []byte(systemAGLPrefix), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := openFile
	t.Cleanup(func() { openFile = prev })
	openFile = func(name string) (fs.File, error) {
		f, err := os.Open(name)
		return closeFailFile{f}, err
	}
	found, err := fileContains(t.Context(), path, systemAGLPrefix)
	if !found || !errors.Is(err, errClose) {
		t.Fatalf("fileContains = %v, %v", found, err)
	}
}
