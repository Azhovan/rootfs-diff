package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Azhovan/rootfs-diff/internal/cli"
)

// Set with -ldflags '-X main.version=...'; ordinary builds report "dev".
var version = "dev"

func main() {
	// Report broken pipes as I/O failures with exit 2 instead of signal exit 141.
	signal.Ignore(syscall.SIGPIPE)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, cli.Options{Version: version})
	stop()
	os.Exit(code)
}
