package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/sergii-ziborov/treestamp/cmd/treestamp/internal/cmdroot"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(cmdroot.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
