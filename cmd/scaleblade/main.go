// Command scaleblade is an unofficial CLI for the Scaleblade portal API.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/tcpsystems/scaleblade/internal/buildinfo"
	"github.com/tcpsystems/scaleblade/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, buildinfo.Version))
}
