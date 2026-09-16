package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/sergii-ziborov/treestamp"
)

func main() {
	root := flag.String("root", ".", "repository root to scan")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, *root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, root string) error {
	report, err := treestamp.ScanWith(ctx, root, treestamp.WithExtensions("go"))
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(os.Stdout, "files=%d complete=%t revision=%s\n", len(report.Files), report.Complete, report.Revision); err != nil {
		return err
	}
	if len(report.Files) == 0 {
		return nil
	}
	why, err := treestamp.Explain(root, report.Files[0].Relative)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "explain %s %s\n", why.Relative, why.Outcome)
	return err
}
