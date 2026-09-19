package walkcfg

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sergii-ziborov/treestamp/internal/walk"
)

func isDirOrLinkToDir(path string, d fs.DirEntry) bool {
	if d == nil {
		return false
	}
	if d.IsDir() {
		return true
	}
	if d.Type()&os.ModeSymlink == 0 {
		return false
	}
	info, err := statEntry(path, d)
	return err == nil && info.IsDir()
}

func statEntry(path string, d fs.DirEntry) (fs.FileInfo, error) {
	if cached, ok := d.(interface{ Stat() (fs.FileInfo, error) }); ok {
		return cached.Stat()
	}
	return os.Stat(path)
}

// IgnoreDuplicateDirs calls fn for every directory, including aliases,
// but does not descend a directory object that was already traversed.
// A directory symlink that is not a duplicate requests ErrTraverseLink.
func IgnoreDuplicateDirs(fn fs.WalkDirFunc) fs.WalkDirFunc {
	filter := NewEntryFilter()
	return func(path string, d fs.DirEntry, err error) error {
		cbErr := fn(path, d, err)
		if cbErr != nil {
			if cbErr != filepath.SkipDir && isDirOrLinkToDir(path, d) {
				filter.Entry(path, d)
			}
			return cbErr
		}
		if !isDirOrLinkToDir(path, d) {
			return nil
		}
		if filter.Entry(path, d) {
			return fs.SkipDir
		}
		if d.Type()&os.ModeSymlink != 0 {
			return walk.ErrTraverseLink
		}
		return nil
	}
}

// IgnoreDuplicateFiles drops a second delivery of the same filesystem
// object. Independent files with the same bytes are kept.
func IgnoreDuplicateFiles(fn fs.WalkDirFunc) fs.WalkDirFunc {
	filter := NewEntryFilter()
	return func(path string, d fs.DirEntry, err error) error {
		if err == nil && d != nil && filter.Entry(path, d) {
			if isDirOrLinkToDir(path, d) {
				return fs.SkipDir
			}
			return nil
		}
		cbErr := fn(path, d, err)
		if cbErr == nil && d != nil && d.Type()&os.ModeSymlink != 0 && isDirOrLinkToDir(path, d) {
			return walk.ErrTraverseLink
		}
		return cbErr
	}
}

// IgnorePermissionErrors swallows fs.ErrPermission from the walk.
func IgnorePermissionErrors(fn fs.WalkDirFunc) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil && os.IsPermission(err) {
			return nil
		}
		return fn(path, d, err)
	}
}
