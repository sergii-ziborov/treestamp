// Package godirwalk is a Treestamp-backed compatibility entry for
// karrick/godirwalk v1.17.0. Switch the import; do not add that
// module as a runtime dependency.
package godirwalk

import (
	"errors"
	"io/fs"

	"github.com/sergii-ziborov/treestamp"
)

// ErrorAction is the ErrorCallback result.
type ErrorAction int

const (
	Halt ErrorAction = iota
	SkipNode
)

// WalkFunc is invoked for every node, including the root.
type WalkFunc func(osPathname string, directoryEntry *Dirent) error

// SkipThis skips the current node without skipping siblings of a file.
var SkipThis = treestamp.SkipThis

// Options matches the godirwalk v1.17.0 walk configuration.
type Options struct {
	ErrorCallback        func(string, error) ErrorAction
	FollowSymbolicLinks  bool
	Unsorted             bool
	Callback             WalkFunc
	PostChildrenCallback WalkFunc
	ScratchBuffer        []byte
	AllowNonDirectory    bool
}

// Walk walks root with the godirwalk option names on Treestamp's engine.
func Walk(root string, options *Options) error {
	if options == nil || options.Callback == nil {
		return errors.New("missing Callback")
	}
	return treestamp.WalkDirs(root, treestamp.DirWalkOptions{
		Callback:             adaptWalk(options.Callback),
		PostChildrenCallback: adaptWalk(options.PostChildrenCallback),
		ErrorCallback:        adaptError(options.ErrorCallback),
		Unsorted:             options.Unsorted,
		FollowSymbolicLinks:  options.FollowSymbolicLinks,
		AllowNonDirectory:    options.AllowNonDirectory,
		ScratchBuffer:        options.ScratchBuffer,
	})
}

func adaptWalk(fn WalkFunc) treestamp.WalkDirFunc {
	if fn == nil {
		return nil
	}
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return fn(path, direntFrom(path, d))
	}
}

func adaptError(fn func(string, error) ErrorAction) func(string, error) error {
	if fn == nil {
		return nil
	}
	return func(path string, err error) error {
		if fn(path, err) == SkipNode {
			return nil
		}
		return err
	}
}
