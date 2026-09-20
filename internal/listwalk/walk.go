package listwalk

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sergii-ziborov/treestamp/internal/dirread"
)

// Config controls a callback walk used by WalkDirs and Walk.
type Config struct {
	After                   fs.WalkDirFunc
	SkipFiles, TraverseLink error
	SkipThis                error
	ToSlash, ContentsFirst  bool
	Sort, Follow            bool
	RequireDirectory        bool
	Scratch                 []byte
	MaxOpen                 int
	Context                 context.Context
	OnLink                  func(path, name string, depth int, ancestors []string) (bool, error)
}

type frame struct {
	path   string
	depth  int
	dents  []dirread.Record
	index  int
	ready  bool
	done   bool
	scan   *dirread.Scanner
	chunks [][]Entry
	cur    []Entry
}

var errStop = errors.New("listwalk stop")

// Walk visits root then descendants. Sort uses lexical names; ContentsFirst
// keeps files before directories after that order. Without those, children
// stream from a directory scanner so stop can close the listing early.
func Walk(root string, fn fs.WalkDirFunc, cfg Config) error {
	if cfg.Context != nil {
		if err := cfg.Context.Err(); err != nil {
			return err
		}
	}
	start, descend, err := Prepare(root, fn, cfg)
	if err != nil || !descend {
		return err
	}
	return walkChildren(start, fn, cfg)
}

// Prepare yields the root entry and reports whether to descend.
func Prepare(root string, fn fs.WalkDirFunc, cfg Config) (string, bool, error) {
	start := filepath.Clean(root)
	if start == "" {
		start = "."
	}
	info, err := os.Lstat(start)
	if err != nil {
		return "", false, fn(Show(start, cfg.ToSlash), nil, err)
	}
	if cfg.RequireDirectory && info.Mode()&os.ModeDir == 0 {
		return "", false, fmt.Errorf("cannot Walk non-directory: %s", start)
	}
	entry := Own(info.Name(), start, info.Mode().Type(), 0, info)
	cbErr := Deliver(fn, entry, cfg.ToSlash)
	typ := entry.typ
	if cbErr != nil {
		if errors.Is(cbErr, fs.SkipAll) || errors.Is(cbErr, fs.SkipDir) || skipThis(cfg, cbErr) {
			return "", false, nil
		}
		return "", false, cbErr
	}
	return start, shouldDescend(cfg, start, typ), nil
}

func shouldDescend(cfg Config, path string, typ fs.FileMode) bool {
	if typ.IsDir() && typ&os.ModeSymlink == 0 {
		return true
	}
	if !cfg.Follow || typ&os.ModeSymlink == 0 {
		return false
	}
	target, err := os.Stat(path)
	return err == nil && target.IsDir()
}

func skipThis(cfg Config, err error) bool {
	return cfg.SkipThis != nil && errors.Is(err, cfg.SkipThis)
}

func walkChildren(root string, fn fs.WalkDirFunc, cfg Config) error {
	st := &state{root: root, fn: fn, cfg: cfg}
	return st.run()
}
