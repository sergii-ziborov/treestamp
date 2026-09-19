// Command treestamp scans a repository tree with the Treestamp library:
// select files, save a baseline, explain a skip, and verify the next tree.
//
//	go install github.com/sergii-ziborov/treestamp/cmd/treestamp@v0.1.4
//
// Write the manifest outside the scan root (a sibling directory can live
// in the same Git repo). Then verify that tree later:
//
//	treestamp scan ./cmd/treestamp --ext go --json --output ./baselines/cli.tstamp.json
//	treestamp verify ./baselines/cli.tstamp.json --root ./cmd/treestamp
//
// List selected names without hashing, or ask why a path was dropped:
//
//	treestamp paths . --ext go --null | xargs -0 gofmt -l
//	treestamp explain skip.txt --root . --ext go
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
