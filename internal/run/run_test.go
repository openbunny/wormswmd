package run

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCommandSuccessAndFailure(t *testing.T) {
	out, err := Command(t.Context(), "/usr/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("stdout = %q", out)
	}
	_, err = Command(t.Context(), "/bin/sh", "-c", "echo fail >&2; exit 3")
	var exitErr *exec.ExitError
	if err == nil || !strings.Contains(err.Error(), "fail") || !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("error = %v", err)
	}
}

func TestCommandKeepsToolOutput(t *testing.T) {
	out, err := Command(t.Context(), "/bin/sh", "-c", "dd if=/dev/zero bs=1024 count=8 status=none")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 8*1024 {
		t.Fatalf("stdout length = %d", len(out))
	}
}

func TestCommandCapsOutput(t *testing.T) {
	_, err := Command(t.Context(), "/bin/sh", "-c", "dd if=/dev/zero bs=1024 count=8 status=none; exit 1")
	if err == nil {
		t.Fatal("expected failure")
	}
	if len(err.Error()) > maxToolOutput+256 {
		t.Fatalf("error length = %d", len(err.Error()))
	}
}

func TestCommandDeadline(t *testing.T) {
	previous := toolTimeout
	toolTimeout = 200 * time.Millisecond
	t.Cleanup(func() {
		toolTimeout = previous
	})
	_, err := Command(context.Background(), "/bin/sleep", "5")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
}
