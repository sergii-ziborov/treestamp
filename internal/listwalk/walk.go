package listwalk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Config controls an unsorted callback walk.
type Config struct {
	After                   fs.WalkDirFunc
	SkipFiles, TraverseLink error
	ToSlash, ContentsFirst  bool
	OnLink                  func(path, name string, depth int, ancestors []string) (bool, error)
}

type frame struct {
	path  string
	depth int
	dents []os.DirEntry
	index int
	ready bool
}

var errStop = errors.New("listwalk stop")

// Walk visits root then descendants. Order is filesystem order unless ContentsFirst.
func Walk(root string, fn fs.WalkDirFunc, cfg Config) error {
	abs, descend, err := Prepare(root, fn, cfg.ToSlash)
	if err != nil || !descend {
		return err
	}
	return walkChildren(abs, fn, cfg)
}

// Prepare yields the root entry and reports whether to descend.
func Prepare(root string, fn fs.WalkDirFunc, toSlash bool) (string, bool, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", false, fn(root, nil, err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", false, fn(Show(abs, toSlash), nil, err)
	}
	entry := Acquire(info.Name(), abs, info.Mode().Type(), 0, info)
	cbErr := Call(fn, entry, toSlash)
	typ := entry.typ
	Release(entry)
	if cbErr != nil {
		if errors.Is(cbErr, fs.SkipAll) || errors.Is(cbErr, fs.SkipDir) {
			return "", false, nil
		}
		return "", false, cbErr
	}
	if typ.IsDir() {
		return abs, true, nil
	}
	if typ&os.ModeSymlink == 0 {
		return abs, false, nil
	}
	target, statErr := os.Stat(abs)
	return abs, statErr == nil && target.IsDir(), nil
}

func walkChildren(root string, fn fs.WalkDirFunc, cfg Config) error {
	frames := []frame{{path: root}}
	skip := ""
	for len(frames) > 0 {
		top := &frames[len(frames)-1]
		if err := fill(top, fn, cfg); err != nil {
			if errors.Is(err, errStop) {
				return nil
			}
			return err
		}
		if top.index >= len(top.dents) {
			if err := finish(root, fn, cfg, top, &skip, &frames); err != nil {
				if errors.Is(err, errStop) {
					return nil
				}
				return err
			}
			continue
		}
		dent := top.dents[top.index]
		top.index++
		if err := visit(fn, cfg, top, dent, &frames, &skip); err != nil {
			if errors.Is(err, errStop) {
				return nil
			}
			return err
		}
	}
	return nil
}
