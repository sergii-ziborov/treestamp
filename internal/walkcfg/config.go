package walkcfg

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"sync"

	"github.com/sergii-ziborov/treestamp/internal/walk"
)

type Config struct {
	Follow, Sort, ToSlash, ContentsFirst, DirsFirst bool
	FollowOutside, KeepSkipAll                      bool
	NumWorkers, MaxDepth, SortMode                  int
}

func Serial(root string, cfg Config, fn fs.WalkDirFunc) error {
	if !cfg.Follow && cfg.MaxDepth == 0 && !cfg.Sort {
		return walk.WalkCallbackHooks(root, fn, walk.CallbackOptions{
			ToSlash: cfg.ToSlash, ContentsFirst: cfg.ContentsFirst, FollowOutside: cfg.FollowOutside,
		})
	}
	opts := walk.DefaultOptions()
	opts.FollowLinks = cfg.Follow
	opts.FollowOutside = cfg.FollowOutside
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
	return Drain(walker, fn, cfg.ToSlash, cfg.KeepSkipAll)
}

func Parallel(root string, cfg Config, fn fs.WalkDirFunc) error {
	workers := cfg.NumWorkers
	if workers < 0 {
		workers = 0
	}
	if !cfg.Follow && cfg.MaxDepth == 0 && !cfg.ContentsFirst && !cfg.DirsFirst {
		return walk.WalkCallbackParallelOpts(root, fn, walk.ParallelCallback{
			Workers: workers, ToSlash: cfg.ToSlash, LocalSort: cfg.SortMode,
			KeepSkipAll: cfg.KeepSkipAll, FollowOutside: cfg.FollowOutside,
		})
	}
	opts := walk.DefaultOptions()
	opts.FollowLinks = cfg.Follow
	opts.FollowOutside = cfg.FollowOutside
	opts.LocalSort = cfg.SortMode
	opts.ContentsFirst, opts.DirsFirst = cfg.ContentsFirst, cfg.DirsFirst
	if cfg.MaxDepth > 0 {
		d := cfg.MaxDepth
		opts.MaxDepth = &d
	}
	hooks := &parallelHooks{cfg: cfg, fn: fn, skipFiles: map[string]struct{}{}}
	_, err := walk.NewParallelWalker(root).Options(opts).WithParallelism(workers).Visit(hooks.apply)
	if hooks.callbackErr != nil {
		return hooks.callbackErr
	}
	return err
}

type parallelHooks struct {
	cfg         Config
	fn          fs.WalkDirFunc
	mu          sync.Mutex
	skipFiles   map[string]struct{}
	callbackErr error
}

func (h *parallelHooks) apply(ev walk.WalkEvent) walk.WalkControl {
	if ev.Err != nil {
		return h.control(h.fn(ev.Err.Path, nil, ev.Err), filepath.Dir(ev.Err.Path))
	}
	path := ev.Entry.Path()
	if h.cfg.ToSlash {
		path = filepath.ToSlash(path)
	}
	if ev.Entry.IsFile() && h.skipping(filepath.Dir(ev.Entry.Path())) {
		return walk.WalkContinue
	}
	cbErr := h.fn(path, walk.NewDirEntry(ev.Entry), nil)
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
	return h.control(cbErr, parent)
}

func (h *parallelHooks) skipping(parent string) bool {
	h.mu.Lock()
	_, skip := h.skipFiles[parent]
	h.mu.Unlock()
	return skip
}

func (h *parallelHooks) control(cbErr error, parent string) walk.WalkControl {
	switch {
	case errors.Is(cbErr, fs.SkipDir):
		return walk.WalkSkip
	case errors.Is(cbErr, fs.SkipAll):
		if h.cfg.KeepSkipAll {
			h.mu.Lock()
			if h.callbackErr == nil {
				h.callbackErr = fs.SkipAll
			}
			h.mu.Unlock()
		}
		return walk.WalkQuit
	case errors.Is(cbErr, walk.ErrSkipFiles):
		h.mu.Lock()
		h.skipFiles[parent] = struct{}{}
		h.mu.Unlock()
		return walk.WalkContinue
	case cbErr == nil:
		return walk.WalkContinue
	default:
		h.mu.Lock()
		if h.callbackErr == nil {
			h.callbackErr = cbErr
		}
		h.mu.Unlock()
		return walk.WalkQuit
	}
}

type walker interface {
	Next() (*walk.WalkEntry, error)
	Close() error
}

func Drain(w walker, fn fs.WalkDirFunc, toSlash bool, keepSkipAll bool) error {
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
				if keepSkipAll {
					return fs.SkipAll
				}
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
