// Package treestamp is a native Go port of Weavatrix Scan.
//
// The repository is the personal public project of Sergii Ziborov
// (github.com/sergii-ziborov/treestamp). It is not published from the
// Weavatrix or EdgeHawk organizations.
//
// Walk, select, hash, cache, and incremental surfaces are implemented. This
// is still not a claim that every rust differential and official bench is
// closed.
package treestamp

import (
	"io/fs"

	"github.com/sergii-ziborov/treestamp/internal/walk"
	"github.com/sergii-ziborov/treestamp/internal/walkfs"
)

type (
	DirEntry = walk.DirEntry
	FSEntry  = walkfs.Entry
	FSWalker = walkfs.Walker
)

// StatDirEntry returns cached os.Stat metadata when entry supports it.
func StatDirEntry(path string, entry fs.DirEntry) (fs.FileInfo, error) {
	return walk.StatDirEntry(path, entry)
}

// DirEntryDepth returns callback depth or -1 for another DirEntry type.
func DirEntryDepth(entry fs.DirEntry) int {
	return walk.DirEntryDepth(entry)
}

// NewFSWalker constructs a deterministic pull walker over an arbitrary fs.FS.
func NewFSWalker(fsys fs.FS, root string) (*FSWalker, error) {
	return walkfs.New(fsys, root)
}

// WalkFS walks an arbitrary fs.FS with fs.WalkDir-compatible callback control.
func WalkFS(fsys fs.FS, root string, fn fs.WalkDirFunc) error {
	return walkfs.Walk(fsys, root, fn)
}
