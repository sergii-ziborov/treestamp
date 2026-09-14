package walk

import (
	"path/filepath"

	"github.com/sergii-ziborov/treestamp/internal/platform"
)

// WalkSkipReason explains why a directory is yielded but not descended.
type WalkSkipReason int

const (
	SkipNone WalkSkipReason = iota
	SkipMaxDepth
	SkipFileSystemBoundary
	SkipPathEscape
	SkipSymlinkLoop
)

func (r WalkSkipReason) String() string {
	switch r {
	case SkipMaxDepth:
		return "max_depth"
	case SkipFileSystemBoundary:
		return "filesystem_boundary"
	case SkipPathEscape:
		return "path_escape"
	case SkipSymlinkLoop:
		return "symlink_loop"
	default:
		return ""
	}
}

// FileVersion is snapshot evidence captured with metadata when requested.
type FileVersion struct {
	ModifiedNS *uint64
	ChangedNS  *uint64
	Identity   *platform.Identity
}

// WalkEntry is one native filesystem record.
type WalkEntry struct {
	root    string
	path    string
	depth   int
	isFile  bool
	isDir   bool
	symlink bool
	bytes   *uint64
	version *FileVersion
	hidden  *bool
	dirID   *platform.Identity
	skip    WalkSkipReason
}

func (e *WalkEntry) Path() string { return e.path }

func (e *WalkEntry) RelativePath() string {
	if e.path == "" || e.root == "" {
		return ""
	}
	if filepath.Clean(e.path) == filepath.Clean(e.root) {
		return ""
	}
	rel, err := filepath.Rel(e.root, e.path)
	if err != nil || rel == "." {
		return ""
	}
	return rel
}

func (e *WalkEntry) FileName() string {
	base := filepath.Base(e.path)
	if base == "." || base == string(filepath.Separator) {
		return e.path
	}
	return base
}

func (e *WalkEntry) Depth() int            { return e.depth }
func (e *WalkEntry) IsFile() bool          { return e.isFile }
func (e *WalkEntry) IsDir() bool           { return e.isDir }
func (e *WalkEntry) IsSymlink() bool       { return e.symlink }
func (e *WalkEntry) Bytes() *uint64        { return e.bytes }
func (e *WalkEntry) Version() *FileVersion { return e.version }
func (e *WalkEntry) SkipReason() WalkSkipReason {
	return e.skip
}

func (e *WalkEntry) hasSkip() bool {
	return e.skip != SkipNone
}
