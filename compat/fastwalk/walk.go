// Package fastwalk is a Treestamp-backed compatibility entry for
// charlievieth/fastwalk v1.0.14. Switch the import; do not add that
// module as a runtime dependency.
package fastwalk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/sergii-ziborov/treestamp"
)

var (
	ErrTraverseLink = errors.New("fastwalk: traverse symlink, assuming target is a directory")
	ErrSkipFiles    = errors.New("fastwalk: skip remaining files in directory")
	SkipDir         = fs.SkipDir
)

type WalkDirFunc = fs.WalkDirFunc

type SortMode uint32

const (
	SortNone SortMode = iota
	SortLexical
	SortFilesFirst
	SortDirsFirst
)

func (s SortMode) String() string {
	switch s {
	case SortNone:
		return "None"
	case SortLexical:
		return "Lexical"
	case SortFilesFirst:
		return "FilesFirst"
	case SortDirsFirst:
		return "DirsFirst"
	default:
		return "SortMode"
	}
}

func DefaultNumWorkers() int {
	n := runtime.GOMAXPROCS(-1)
	if n < 4 {
		return 4
	}
	if runtime.GOOS == "darwin" {
		switch {
		case n <= 8:
			return 4
		case n <= 10:
			return 6
		default:
			return 10
		}
	}
	if n > 32 {
		return 32
	}
	return n
}

func DefaultToSlash() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	_, ok := os.LookupEnv("MSYSTEM")
	return ok
}

var DefaultConfig = Config{
	ToSlash:    DefaultToSlash(),
	NumWorkers: DefaultNumWorkers(),
}

type Config struct {
	Follow     bool
	ToSlash    bool
	Sort       SortMode
	NumWorkers int
	MaxDepth   int
}

func (c *Config) Copy() *Config {
	dupe := new(Config)
	if c != nil {
		*dupe = *c
	}
	return dupe
}

func Walk(conf *Config, root string, walkFn fs.WalkDirFunc) error {
	if _, err := os.Stat(root); err != nil {
		return err
	}
	if conf == nil {
		c := DefaultConfig
		conf = &c
	}
	if conf.ToSlash {
		root = filepath.ToSlash(root)
	}
	workers := conf.NumWorkers
	if workers <= 0 {
		workers = DefaultNumWorkers()
	}
	return treestamp.WalkWithConfig(root, treestamp.Config{
		Follow:        conf.Follow,
		FollowOutside: true,
		ToSlash:       conf.ToSlash,
		NumWorkers:    workers,
		MaxDepth:      conf.MaxDepth,
		SortMode:      int(conf.Sort),
		KeepSkipAll:   true,
	}, adapt(walkFn))
}

func adapt(fn fs.WalkDirFunc) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		cbErr := fn(path, d, err)
		switch {
		case errors.Is(cbErr, ErrSkipFiles):
			return treestamp.ErrSkipFiles
		case errors.Is(cbErr, ErrTraverseLink):
			return treestamp.ErrTraverseLink
		default:
			return cbErr
		}
	}
}
