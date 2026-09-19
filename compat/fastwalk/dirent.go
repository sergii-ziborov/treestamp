package fastwalk

import (
	"io/fs"

	"github.com/sergii-ziborov/treestamp"
)

// DirEntry is the callback entry: cached Info/Stat and Depth.
type DirEntry interface {
	fs.DirEntry
	Stat() (fs.FileInfo, error)
	Depth() int
}

func StatDirEntry(path string, de fs.DirEntry) (fs.FileInfo, error) {
	return treestamp.StatDirEntry(path, de)
}

func DirEntryDepth(de fs.DirEntry) int {
	return treestamp.DirEntryDepth(de)
}
