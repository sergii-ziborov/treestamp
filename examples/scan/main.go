package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/sergii-ziborov/treestamp"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	ctx := context.Background()
	paths, err := treestamp.ScanPaths(ctx, root)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("paths %d\n", len(paths))

	scanner, err := treestamp.NewScanner(root)
	if err != nil {
		log.Fatal(err)
	}
	report, err := scanner.Scan(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", report.Summary())
	fmt.Printf("legacy %s\n", report.Revision)
	fmt.Printf("tree   %s\n", treestamp.SnapshotFromFiles(report.Files).TreeRevision())
	if len(report.Files) > 0 {
		why, err := scanner.Explain(report.Files[0].Relative)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("explain %s %s %s\n", why.Relative, why.Outcome, why.Reason)
	}
}
