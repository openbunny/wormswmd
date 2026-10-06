package cmd

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
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
			slog.Info("resolved the app", "app", "Worms W.M.D.app")
			slog.Debug("ran codesign")
			slog.Warn("free space is below the minimum")
			text := buf.String()
			if strings.Contains(text, "resolved the app") != tc.wantInfo || strings.Contains(text, "ran codesign") != tc.wantDebug || !strings.Contains(text, "free space is below the minimum") {
				t.Fatalf("log = %q", text)
			}
		})
	}
}

func TestVerboseAndQuietConflict(t *testing.T) {
	root := New()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version", "--verbose", "--quiet"})
	if err := root.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("Execute = %v", err)
	}
}
