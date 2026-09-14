package walk

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sergii-ziborov/treestamp/internal/platform"
)

func (w *Walker) visitPlain(path string, depth int, mode os.FileMode) *WalkEntry {
	symlink := mode&os.ModeSymlink != 0
	isFile := !symlink && mode.IsRegular()
	isDir := !symlink && mode.IsDir()
	if isDir {
		w.pending = &pendingDir{path: path, depth: depth}
	}
	return &WalkEntry{
		root:    w.root,
		path:    path,
		depth:   depth,
		isFile:  isFile,
		isDir:   isDir,
		symlink: symlink,
	}
}

func (w *Walker) visit(
	path string,
	depth int,
	mode os.FileMode,
	bytes *uint64,
	version *FileVersion,
	hidden *bool,
) (*WalkEntry, *WalkError) {
	symlink := mode&os.ModeSymlink != 0
	isFile := mode.IsRegular()
	isDir := mode.IsDir()
	var skip WalkSkipReason
	var dirID *platform.Identity

	var target os.FileInfo
	if symlink && w.options.FollowLinks {
		info, err := os.Stat(path)
		if err != nil {
			return nil, walkErr(path, depth, OpReadMetadata, err)
		}
		target = info
		isFile = info.Mode().IsRegular()
		isDir = info.IsDir()
		if w.options.CollectMetadata && isFile {
			size := uint64(info.Size())
			bytes = &size
			ver := versionFromInfo(path, info)
			version = &ver
			h := platform.HiddenFromInfo(path, info)
			hidden = &h
		}
	}

	if symlink && !w.options.FollowLinks {
		isFile = false
		isDir = false
	} else if symlink || (isDir && (w.options.SameFileSystem || w.options.FollowLinks)) {
		canonical := w.root
		if depth != 0 {
			abs, absErr := filepath.Abs(path)
			if absErr != nil {
				return nil, walkErr(path, depth, OpCanonicalize, absErr)
			}
			resolved, resErr := filepath.EvalSymlinks(abs)
			if resErr != nil {
				return nil, walkErr(path, depth, OpCanonicalize, resErr)
			}
			canonical = resolved
		}
		if !containedBy(w.root, canonical) {
			skip = SkipPathEscape
		} else if isDir {
			var info platform.Info
			if depth == 0 && w.rootInfo != nil {
				info = *w.rootInfo
			} else {
				meta := target
				if meta == nil {
					st, stErr := os.Stat(path)
					if stErr != nil {
						return nil, walkErr(path, depth, OpReadMetadata, stErr)
					}
					meta = st
				}
				got, infoErr := platform.DirectoryInfo(canonical)
				if infoErr != nil {
					return nil, walkErr(path, depth, OpReadMetadata, infoErr)
				}
				_ = meta
				info = got
			}
			if w.options.SameFileSystem && w.rootFS != nil && info.FileSystem != *w.rootFS {
				skip = SkipFileSystemBoundary
			}
			if w.options.FollowLinks {
				id := info.Identity
				dirID = &id
			}
		}
	}

	if isDir && skip == SkipNone {
		if w.options.atOrBeyondMaxDepth(depth) {
			skip = SkipMaxDepth
		} else if w.options.FollowLinks && dirID != nil && depth != 0 && w.active[*dirID] > 0 {
			skip = SkipSymlinkLoop
		}
	}
	if isDir && skip == SkipNone {
		w.pending = &pendingDir{path: path, depth: depth, identity: dirID}
	}
	return &WalkEntry{
		root:    w.root,
		path:    path,
		depth:   depth,
		isFile:  isFile,
		isDir:   isDir,
		symlink: symlink,
		bytes:   bytes,
		version: version,
		hidden:  hidden,
		dirID:   dirID,
		skip:    skip,
	}, nil
}

func containedBy(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}
