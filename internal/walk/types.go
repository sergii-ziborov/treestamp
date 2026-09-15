package walk

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/sergii-ziborov/treestamp/internal/platform"
)

// ErrorPolicy selects whether a local walk error stops the walker.
type ErrorPolicy int

const (
	ErrorContinue ErrorPolicy = iota
	ErrorAbort
)

// RootSymlinkPolicy controls whether the supplied root may itself be a symlink.
type RootSymlinkPolicy int

const (
	RootFollow RootSymlinkPolicy = iota
	RootReject
)

// WalkOptions is the serial traversal policy.
type WalkOptions struct {
	MinDepth          int
	MaxDepth          *int
	MaxOpen           int
	SameFileSystem    bool
	FollowLinks       bool
	CollectMetadata   bool
	ErrorPolicy       ErrorPolicy
	RootSymlinkPolicy RootSymlinkPolicy
}

const DefaultMaxOpen = 64

func DefaultOptions() WalkOptions {
	return WalkOptions{MaxOpen: DefaultMaxOpen, ErrorPolicy: ErrorContinue, RootSymlinkPolicy: RootFollow}
}

func (o WalkOptions) Normalize() WalkOptions {
	if o.MaxOpen <= 0 {
		o.MaxOpen = 1
	}
	if o.MaxDepth != nil && o.MinDepth > *o.MaxDepth {
		o.MinDepth = *o.MaxDepth
	}
	return o
}

func (o WalkOptions) atOrBeyondMaxDepth(depth int) bool {
	return o.MaxDepth != nil && depth >= *o.MaxDepth
}

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

type FileVersion struct {
	ModifiedNS *uint64
	ChangedNS  *uint64
	Identity   *platform.Identity
}

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
	stat    *fileInfoCache
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

func (e *WalkEntry) Depth() int                 { return e.depth }
func (e *WalkEntry) IsFile() bool               { return e.isFile }
func (e *WalkEntry) IsDir() bool                { return e.isDir }
func (e *WalkEntry) IsSymlink() bool            { return e.symlink }
func (e *WalkEntry) Bytes() *uint64             { return e.bytes }
func (e *WalkEntry) Version() *FileVersion      { return e.version }
func (e *WalkEntry) SkipReason() WalkSkipReason { return e.skip }
func (e *WalkEntry) hasSkip() bool              { return e.skip != SkipNone }

// Stat returns cached os.Stat metadata for the entry path.
func (e *WalkEntry) Stat() (fs.FileInfo, error) {
	if e == nil || e.path == "" {
		return nil, invalidStatPath("")
	}
	if e.stat == nil {
		return os.Stat(e.path)
	}
	return e.stat.get(e.path)
}

type fileInfoCache struct {
	once sync.Once
	info fs.FileInfo
	err  error
}

func newFileInfoCache() *fileInfoCache {
	return &fileInfoCache{}
}

func (c *fileInfoCache) load(info fs.FileInfo, err error) {
	c.once.Do(func() {
		c.info, c.err = info, err
	})
}

func (c *fileInfoCache) get(path string) (fs.FileInfo, error) {
	c.once.Do(func() {
		c.info, c.err = os.Stat(path)
	})
	return c.info, c.err
}

// DirEntry extends fs.DirEntry with cached target Stat and walk depth.
type DirEntry interface {
	fs.DirEntry
	Stat() (fs.FileInfo, error)
	Depth() int
}

type callbackDirEntry struct {
	fs.DirEntry
	source *WalkEntry
}

func (d *callbackDirEntry) Stat() (fs.FileInfo, error) {
	if d.Type()&os.ModeSymlink == 0 {
		return d.Info()
	}
	return d.source.Stat()
}

func (d *callbackDirEntry) Depth() int {
	return d.source.Depth()
}

// NewDirEntry constructs the cached callback entry for a walk event.
func NewDirEntry(entry *WalkEntry, info fs.FileInfo) DirEntry {
	if entry.stat == nil {
		entry.stat = newFileInfoCache()
	}
	if info.Mode()&os.ModeSymlink == 0 {
		entry.stat.load(info, nil)
	}
	return &callbackDirEntry{DirEntry: fs.FileInfoToDirEntry(info), source: entry}
}

// StatDirEntry uses cached target metadata when entry supports it.
func StatDirEntry(path string, entry fs.DirEntry) (fs.FileInfo, error) {
	if entry == nil {
		return nil, invalidStatPath(path)
	}
	if entry.Type()&os.ModeSymlink == 0 {
		return entry.Info()
	}
	if cached, ok := entry.(interface{ Stat() (fs.FileInfo, error) }); ok {
		return cached.Stat()
	}
	return os.Stat(path)
}

func invalidStatPath(path string) error {
	return &os.PathError{Op: "stat", Path: path, Err: fs.ErrInvalid}
}

// DirEntryDepth returns callback depth or -1 for another DirEntry type.
func DirEntryDepth(entry fs.DirEntry) int {
	if entry, ok := entry.(interface{ Depth() int }); ok {
		return entry.Depth()
	}
	return -1
}

type WalkOperation int

const (
	OpCanonicalize WalkOperation = iota
	OpReadDirectory
	OpReadEntry
	OpReadMetadata
	OpScheduleWorker
)

func (o WalkOperation) String() string {
	switch o {
	case OpCanonicalize:
		return "canonicalize"
	case OpReadDirectory:
		return "read directory"
	case OpReadEntry:
		return "read entry"
	case OpReadMetadata:
		return "read metadata"
	case OpScheduleWorker:
		return "schedule worker"
	default:
		return "walk"
	}
}

type WalkError struct {
	Path      string
	Depth     int
	Operation WalkOperation
	Err       error
}

func (e *WalkError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s at depth %d for %s: %v", e.Operation, e.Depth, e.Path, e.Err)
}

func (e *WalkError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func walkErr(path string, depth int, op WalkOperation, err error) *WalkError {
	return &WalkError{Path: path, Depth: depth, Operation: op, Err: err}
}

func isEOF(err error) bool {
	return err == io.EOF || err == fs.ErrClosed
}

type WalkControl int

const (
	WalkContinue WalkControl = iota
	WalkSkip
	WalkQuit
	// WalkTraverseLink requests traversal of the callback's directory symlink.
	WalkTraverseLink
)

// ErrTraverseLink requests traversal of the symlink passed to a callback.
var ErrTraverseLink = errors.New("treestamp: traverse symlink target directory")

type WalkEvent struct {
	Entry *WalkEntry
	Err   *WalkError
}

func (e WalkEvent) IsError() bool { return e.Err != nil }

type ParallelWalkReport struct {
	Entries []*WalkEntry
	Errors  []*WalkError
}

type ParallelVisitReport struct {
	Visited   uint64
	Errors    []*WalkError
	Quit      bool
	Cancelled bool
}

type WalkFunc func(*WalkEntry) error
type ControlFunc func(WalkEvent) WalkControl
