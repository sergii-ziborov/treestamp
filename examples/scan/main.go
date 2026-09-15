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
	report, err := treestamp.Scan(ctx, root)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", report.Summary())
}
