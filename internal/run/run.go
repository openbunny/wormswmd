package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

const maxToolOutput = 1 << 20

var toolTimeout = 10 * time.Minute

type Exec func(ctx context.Context, name string, args ...string) ([]byte, error)

func Command(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if ctxErr := ctx.Err(); err != nil && ctxErr != nil && !errors.Is(err, ctxErr) {
		err = errors.Join(ctxErr, err)
	}
	out := buf.Bytes()
	if len(out) > maxToolOutput {
		out = out[:maxToolOutput]
	}
	if err != nil {
		if len(out) == 0 {
			return nil, fmt.Errorf("run: %s: %w", name, err)
		}
		return out, fmt.Errorf("run: %s: %w: %s", name, err, out)
	}
	return out, nil
}

func Or(fn Exec) Exec {
	if fn == nil {
		return Command
	}
	return fn
}
