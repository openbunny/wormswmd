package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/openbunny/wormswmd/cmd"
)

func main() {
	os.Exit(run())
}

func run() int {
	cmd.UseLogger(os.Stderr, false)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := cmd.New().ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	if exit, ok := errors.AsType[*cmd.ExitError](err); ok {
		if exit.Err != nil {
			fmt.Fprintln(os.Stderr, exit.Err)
		}
		return exit.Code
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}
