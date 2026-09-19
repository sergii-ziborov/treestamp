package walkcfg

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/sergii-ziborov/treestamp/internal/walk"
)

type Config struct {
	Follow, Sort, ToSlash, ContentsFirst, DirsFirst bool
	NumWorkers, MaxDepth                            int
}

func Serial(root string, cfg Config, fn fs.WalkDirFunc) error {
	if !cfg.Follow && cfg.MaxDepth == 0 && !cfg.Sort {
		return walk.WalkCallbackHooks(root, fn, walk.CallbackOptions{ToSlash: cfg.ToSlash, ContentsFirst: cfg.ContentsFirst})
	}
	opts := walk.DefaultOptions()
	opts.FollowLinks = cfg.Follow
	opts.ContentsFirst, opts.DirsFirst = cfg.ContentsFirst, cfg.DirsFirst
	if cfg.MaxDepth > 0 {
		d := cfg.MaxDepth
		opts.MaxDepth = &d
	}
	builder := walk.NewBuilder(root).Options(opts)
	if cfg.ContentsFirst {
		builder = builder.ContentsFirst(true)
	}
	if cfg.Sort {
		builder = builder.SortByFileName()
	}
	walker := builder.Build()
	defer walker.Close()
	return Drain(walker, fn, cfg.ToSlash)
}

func Parallel(root string, cfg Config, fn fs.WalkDirFunc) error {
	workers := cfg.NumWorkers
	if workers < 0 {
		workers = 0
	}
	if !cfg.Follow && cfg.MaxDepth == 0 && !cfg.ContentsFirst && !cfg.DirsFirst {
		return walk.WalkCallbackParallel(root, workers, fn, cfg.ToSlash)
	}
	opts := walk.DefaultOptions()
	opts.FollowLinks = cfg.Follow
	opts.ContentsFirst, opts.DirsFirst = cfg.ContentsFirst, cfg.DirsFirst
	if cfg.MaxDepth > 0 {
		d := cfg.MaxDepth
		opts.MaxDepth = &d
	}
	var mu sync.Mutex
	var callbackErr error
	skipFiles := map[string]struct{}{}
	_, err := walk.NewParallelWalker(root).Options(opts).WithParallelism(workers).Visit(func(ev walk.WalkEvent) walk.WalkControl {
		return applyCallback(ev, cfg, fn, &mu, skipFiles, &callbackErr)
	})
	if callbackErr != nil {
		return callbackErr
	}
	return err
}

func applyCallback(ev walk.WalkEvent, cfg Config, fn fs.WalkDirFunc, mu *sync.Mutex, skipFiles map[string]struct{}, callbackErr *error) walk.WalkControl {
	if ev.Err != nil {
		return control(fn(ev.Err.Path, nil, ev.Err), mu, skipFiles, filepath.Dir(ev.Err.Path), callbackErr)
	}
	path := ev.Entry.Path()
	if cfg.ToSlash {
		path = filepath.ToSlash(path)
	}
	if len(skipFiles) > 0 && ev.Entry.IsFile() {
		parent := filepath.Dir(ev.Entry.Path())
		mu.Lock()
		_, skip := skipFiles[parent]
		mu.Unlock()
		if skip {
			return walk.WalkContinue
		}
	}
	cbErr := fn(path, walk.NewDirEntry(ev.Entry), nil)
	if errors.Is(cbErr, walk.ErrSkipThis) {
		if ev.Entry.IsDir() {
			return walk.WalkSkip
		}
		return walk.WalkContinue
	}
	if errors.Is(cbErr, walk.ErrTraverseLink) && ev.Entry.IsSymlink() {
		return walk.WalkTraverseLink
	}
	parent := ""
	if errors.Is(cbErr, walk.ErrSkipFiles) {
		parent = filepath.Dir(ev.Entry.Path())
	}
	return control(cbErr, mu, skipFiles, parent, callbackErr)
}

func control(cbErr error, mu *sync.Mutex, skipFiles map[string]struct{}, parent string, callbackErr *error) walk.WalkControl {
	switch {
	case errors.Is(cbErr, fs.SkipDir):
		return walk.WalkSkip
	case errors.Is(cbErr, fs.SkipAll):
		return walk.WalkQuit
	case errors.Is(cbErr, walk.ErrSkipFiles):
		mu.Lock()
		skipFiles[parent] = struct{}{}
		mu.Unlock()
		return walk.WalkContinue
	case cbErr == nil:
		return walk.WalkContinue
	default:
		mu.Lock()
		if *callbackErr == nil {
			*callbackErr = cbErr
		}
		mu.Unlock()
		return walk.WalkQuit
	}
}

type walker interface {
	Next() (*walk.WalkEntry, error)
	Close() error
}

func Drain(w walker, fn fs.WalkDirFunc, toSlash bool) error {
	var skipFiles bool
	var skipDir string
	for {
		entry, err := w.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fn("", nil, err)
		}
		path := entry.Path()
		if toSlash {
			path = filepath.ToSlash(path)
		}
		if skipFiles && entry.IsFile() && filepath.Dir(entry.Path()) == skipDir {
			continue
		}
		if err := drainOne(w, fn, path, entry, &skipFiles, &skipDir); err != nil {
			if errors.Is(err, errDrainStop) {
				return nil
			}
			return err
		}
	}
}

func drainOne(w walker, fn fs.WalkDirFunc, path string, entry *walk.WalkEntry, skipFiles *bool, skipDir *string) error {
	switch cbErr := fn(path, walk.NewDirEntry(entry), nil); {
	case errors.Is(cbErr, fs.SkipDir):
		if skipper, ok := w.(interface{ SkipCurrentDir() }); ok {
			skipper.SkipCurrentDir()
		}
	case errors.Is(cbErr, fs.SkipAll):
		return errDrainStop
	case errors.Is(cbErr, walk.ErrSkipThis):
		if entry.IsDir() {
			if skipper, ok := w.(interface{ SkipCurrentDir() }); ok {
				skipper.SkipCurrentDir()
			}
		}
	case errors.Is(cbErr, walk.ErrSkipFiles):
		*skipFiles, *skipDir = true, filepath.Dir(entry.Path())
	case errors.Is(cbErr, walk.ErrTraverseLink) && entry.IsSymlink():
		follower, ok := w.(interface{ TraverseCurrentSymlink() error })
		if !ok {
			return cbErr
		}
		return follower.TraverseCurrentSymlink()
	case cbErr != nil:
		return cbErr
	}
	return nil
}

var errDrainStop = errors.New("drain stop")

func IgnoreDuplicate(fn fs.WalkDirFunc, dirsOnly bool) fs.WalkDirFunc {
	seen := map[string]struct{}{}
	var mu sync.Mutex
	return func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || (dirsOnly && !d.IsDir()) {
			return fn(path, d, err)
		}
		key := filepath.Clean(path)
		if !dirsOnly {
			if info, infoErr := d.Info(); infoErr == nil {
				key = key + "\x00" + info.ModTime().String() + "\x00" + strconv.FormatUint(uint64(info.Size()), 10)
			}
		}
		mu.Lock()
		_, dup := seen[key]
		if !dup {
			seen[key] = struct{}{}
		}
		mu.Unlock()
		if !dup {
			return fn(path, d, err)
		}
		if d.IsDir() {
			return fs.SkipDir
		}
		return nil
	}
}
