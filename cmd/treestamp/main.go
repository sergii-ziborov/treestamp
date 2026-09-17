// Command treestamp scans a repository tree with the Treestamp library:
// select files, save a baseline, explain a skip, and verify the next tree.
//
//	go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.2
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
