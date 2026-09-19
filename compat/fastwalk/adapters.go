package fastwalk

import (
	"io/fs"
	"runtime"

	"github.com/sergii-ziborov/treestamp"
)

type EntryFilter struct {
	inner *treestamp.EntryFilter
}

func NewEntryFilter() *EntryFilter {
	return &EntryFilter{inner: treestamp.NewEntryFilter()}
}

func (e *EntryFilter) Entry(path string, de fs.DirEntry) bool {
	if e == nil || e.inner == nil {
		return false
	}
	seen, ok := e.inner.Observe(path, de)
	if !ok {
		return runtime.GOOS != "windows"
	}
	return seen
}

func IgnoreDuplicateFiles(walkFn fs.WalkDirFunc) fs.WalkDirFunc {
	return adapt(treestamp.IgnoreDuplicateFiles(walkFn))
}

func IgnoreDuplicateDirs(walkFn fs.WalkDirFunc) fs.WalkDirFunc {
	return adapt(treestamp.IgnoreDuplicateDirs(walkFn))
}

func IgnorePermissionErrors(walkFn fs.WalkDirFunc) fs.WalkDirFunc {
	return treestamp.IgnorePermissionErrors(walkFn)
}
