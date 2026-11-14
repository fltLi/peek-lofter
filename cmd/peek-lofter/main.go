package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/fltLi/peek-lofter/internal/cli"
	"github.com/fltLi/peek-lofter/pkg/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	code := cli.Execute(ctx, os.Args[1:], cli.Deps{
		In:      os.Stdin,
		Out:     os.Stdout,
		Err:     os.Stderr,
		IsTTY:   cli.IsTerminal(os.Stdout),
		Version: version.Version,
	})
	os.Exit(code)
}
