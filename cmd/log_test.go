package cmd

import (
	"bytes"
	"strings"
	"testing"
)

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
