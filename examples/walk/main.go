package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/sergii-ziborov/treestamp"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	// Pull walker. For a godirwalk-shaped callback use WalkDirs;
	// see docs/recipes/walk-dirs.md.
	walker, err := treestamp.NewWalker(root)
	if err != nil {
		log.Fatal(err)
	}
	defer walker.Close()
	for {
		entry, err := walker.Next()
		if err == io.EOF {
			return
		}
		if err != nil {
			log.Println(err)
			continue
		}
		fmt.Println(entry.Path())
	}
}
