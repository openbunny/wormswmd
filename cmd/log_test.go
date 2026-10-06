package cmd

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestUseLoggerLevel(t *testing.T) {
	cases := []struct {
		name     string
		verbose  bool
		wantInfo bool
	}{
		{name: "default", verbose: false},
		{name: "verbose", verbose: true, wantInfo: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			prev := slog.Default()
			t.Cleanup(func() { slog.SetDefault(prev) })
			UseLogger(&buf, tc.verbose)
			slog.Info("resolved the app", "app", "Worms W.M.D.app")
			slog.Warn("free space is below the minimum")
			text := buf.String()
			if strings.Contains(text, "resolved the app") != tc.wantInfo || !strings.Contains(text, "free space is below the minimum") {
				t.Fatalf("log = %q", text)
			}
		})
	}
}
