package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"

	"charm.land/fang/v2"

	"github.com/openbunny/wormswmd/cmd"
)

func main() {
	os.Exit(run())
}

func run() int {
	cmd.UseLogger(os.Stderr, cmd.Normal)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := fang.Execute(ctx, cmd.New(),
		fang.WithVersion(cmd.Version()),
		fang.WithErrorHandler(renderError),
	)
	if err == nil {
		return 0
	}
	if exit, ok := errors.AsType[*cmd.ExitError](err); ok {
		return exit.Code
	}
	return 1
}

// renderError styles real errors through fang but stays silent for an
// ExitError that carries only an exit code: a command that already printed its
// result and returns the code alone has nothing left to report.
func renderError(w io.Writer, styles fang.Styles, err error) {
	if exit, ok := errors.AsType[*cmd.ExitError](err); ok && exit.Err == nil {
		return
	}
	fang.DefaultErrorHandler(w, styles, err)
}
