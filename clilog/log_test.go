package clilog

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestUseLoggerLevel(t *testing.T) {
	cases := []struct {
		name      string
		verbosity Verbosity
		wantInfo  bool
		wantDebug bool
	}{
		{name: "quiet", verbosity: Quiet},
		{name: "normal", verbosity: Normal, wantInfo: true},
		{name: "verbose", verbosity: Verbose, wantInfo: true, wantDebug: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			prev := slog.Default()
			t.Cleanup(func() { slog.SetDefault(prev) })
			UseLogger(&buf, tc.verbosity)
			slog.Info("resolved the app")
			slog.Debug("ran codesign")
			slog.Warn("free space is below the minimum")
			text := buf.String()
			if strings.Contains(text, "resolved the app") != tc.wantInfo || strings.Contains(text, "ran codesign") != tc.wantDebug || !strings.Contains(text, "free space is below the minimum") {
				t.Fatalf("log = %q", text)
			}
		})
	}
}

func testRoot(t *testing.T) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "sub", SilenceErrors: true, SilenceUsage: true}
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		slog.Info("resolved the app")
		slog.Debug("ran codesign")
		slog.Warn("free space is below the minimum")
		return nil
	}
	Attach(root)
	return root
}

func TestAttachLevels(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantInfo  bool
		wantDebug bool
	}{
		{name: "default", args: []string{}, wantInfo: true},
		{name: "verbose", args: []string{"--verbose"}, wantInfo: true, wantDebug: true},
		{name: "quiet", args: []string{"--quiet"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := testRoot(t)
			var buf bytes.Buffer
			root.SetErr(&buf)
			root.SetArgs(tc.args)
			if err := root.ExecuteContext(t.Context()); err != nil {
				t.Fatalf("Execute = %v", err)
			}
			text := buf.String()
			if strings.Contains(text, "resolved the app") != tc.wantInfo || strings.Contains(text, "ran codesign") != tc.wantDebug || !strings.Contains(text, "free space is below the minimum") {
				t.Fatalf("log = %q", text)
			}
		})
	}
}

func TestAttachConflicts(t *testing.T) {
	root := testRoot(t)
	root.SetArgs([]string{"--verbose", "--quiet"})
	if err := root.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("Execute = %v", err)
	}
}

func TestAttachChainsPreRun(t *testing.T) {
	root := &cobra.Command{Use: "sub", SilenceErrors: true, SilenceUsage: true}
	var chained bool
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error { chained = true; return nil }
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		slog.Info("resolved the app")
		return nil
	}
	Attach(root)
	var buf bytes.Buffer
	root.SetErr(&buf)
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("Execute = %v", err)
	}
	if !chained || !strings.Contains(buf.String(), "resolved the app") {
		t.Fatalf("chained = %v, log = %q", chained, buf.String())
	}
}
