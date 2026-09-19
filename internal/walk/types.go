package walk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/sergii-ziborov/treestamp/internal/listwalk"
	pathx "github.com/sergii-ziborov/treestamp/internal/path"
	"github.com/sergii-ziborov/treestamp/internal/platform"
)

type ErrorPolicy int

const (
	ErrorContinue ErrorPolicy = iota
	ErrorAbort
)

type RootSymlinkPolicy int

const (
	RootFollow RootSymlinkPolicy = iota
	RootReject
)

type WalkOptions struct {
	MinDepth          int
	MaxDepth          *int
	MaxOpen           int
	SameFileSystem    bool
	FollowLinks       bool
	CollectMetadata   bool
	ErrorPolicy       ErrorPolicy
	RootSymlinkPolicy RootSymlinkPolicy
	ContentsFirst     bool
	DirsFirst         bool
	FollowOutside     bool
	LocalSort         int
}

const (
	LocalSortNone = iota
	LocalSortLexical
	LocalSortFilesFirst
	LocalSortDirsFirst
)

type ParallelCallback struct {
	Workers       int
	LocalSort     int
	ToSlash       bool
	KeepSkipAll   bool
	FollowOutside bool
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
	names := [...]string{"", "max_depth", "filesystem_boundary", "path_escape", "symlink_loop"}
	if int(r) < len(names) {
		return names[r]
	}
	return ""
}

type FileVersion struct {
	ModifiedNS *uint64            `json:"modified_ns,omitempty"`
	ChangedNS  *uint64            `json:"changed_ns,omitempty"`
	Identity   *platform.Identity `json:"identity,omitempty"`
}

func (v FileVersion) Reusable(other FileVersion) bool {
	if v.ModifiedNS == nil || other.ModifiedNS == nil || *v.ModifiedNS != *other.ModifiedNS {
		return false
	}
	sameChg := v.ChangedNS == nil || other.ChangedNS == nil || *v.ChangedNS == *other.ChangedNS
	sameID := v.Identity == nil || other.Identity == nil || v.Identity.Equal(*other.Identity)
	return sameChg && sameID
}

type WalkEntry struct {
	root, path, name, rel         string
	depth                         int
	isFile, isDir, symlink, relOK bool
	bytes                         *uint64
	version                       *FileVersion
	hidden                        *bool
	dirID                         *platform.Identity
	skip                          WalkSkipReason
	dent                          os.DirEntry
	info                          fs.FileInfo
	stat                          *fileInfoCache
	cb                            callbackDirEntry
}

func (e *WalkEntry) Path() string { return e.path }

func (e *WalkEntry) RelativePath() string {
	if e.relOK {
		return e.rel
	}
	e.rel = relativeUnder(e.root, e.path)
	e.relOK = true
	return e.rel
}

func relativeUnder(root, path string) string {
	root, path = pathx.Native(root), pathx.Native(path)
	if path == "" || root == "" || path == root {
		return ""
	}
	if len(path) > len(root) && path[:len(root)] == root && os.IsPathSeparator(path[len(root)]) {
		return path[len(root)+1:]
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return ""
	}
	return rel
}

func (e *WalkEntry) FileName() string {
	if e.name != "" {
		return e.name
	}
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

func (e *WalkEntry) Info() (fs.FileInfo, error) {
	if e == nil || e.path == "" {
		return nil, invalidStatPath("")
	}
	if e.info != nil {
		return e.info, nil
	}
	if e.dent != nil {
		return e.cacheInfo(e.dent.Info())
	}
	return e.cacheInfo(os.Lstat(e.path))
}

func (e *WalkEntry) cacheInfo(info fs.FileInfo, err error) (fs.FileInfo, error) {
	if err != nil {
		return nil, err
	}
	e.info = info
	if info.Mode()&os.ModeSymlink == 0 {
		e.ensureStat().load(info, nil)
	}
	return info, nil
}

func (e *WalkEntry) ensureStat() *fileInfoCache {
	if e.stat == nil {
		e.stat = newFileInfoCache()
	}
	return e.stat
}

func (e *WalkEntry) Stat() (fs.FileInfo, error) {
	if e == nil || e.path == "" {
		return nil, invalidStatPath("")
	}
	return e.ensureStat().get(e.path)
}

type fileInfoCache struct {
	once sync.Once
	info fs.FileInfo
	err  error
}

func newFileInfoCache() *fileInfoCache { return &fileInfoCache{} }

func (c *fileInfoCache) load(info fs.FileInfo, err error) {
	c.once.Do(func() {
		c.info, c.err = info, err
	})
}

func (c *fileInfoCache) get(path string) (fs.FileInfo, error) {
	return c.loadPath(path, false)
}

func (c *fileInfoCache) getLstat(path string) (fs.FileInfo, error) {
	return c.loadPath(path, true)
}

func (c *fileInfoCache) loadPath(path string, lstat bool) (fs.FileInfo, error) {
	c.once.Do(func() {
		if lstat {
			c.info, c.err = os.Lstat(path)
			return
		}
		c.info, c.err = os.Stat(path)
	})
	return c.info, c.err
}

type DirEntry interface {
	fs.DirEntry
	Stat() (fs.FileInfo, error)
	Depth() int
}

type callbackDirEntry struct {
	source *WalkEntry
}

func (d *callbackDirEntry) Name() string { return d.source.FileName() }

func (d *callbackDirEntry) IsDir() bool {
	if d.source.dent != nil {
		return d.source.dent.IsDir()
	}
	if d.source.info != nil {
		return d.source.info.IsDir() && d.source.info.Mode()&os.ModeSymlink == 0
	}
	return d.source.isDir
}

func (d *callbackDirEntry) Type() fs.FileMode {
	if d.source.dent != nil {
		return d.source.dent.Type()
	}
	if d.source.info != nil {
		return d.source.info.Mode().Type()
	}
	if d.source.symlink {
		return os.ModeSymlink
	}
	if d.source.isDir {
		return os.ModeDir
	}
	return 0
}

func (d *callbackDirEntry) Info() (fs.FileInfo, error) { return d.source.Info() }

func (d *callbackDirEntry) Stat() (fs.FileInfo, error) {
	if d.Type()&os.ModeSymlink == 0 {
		return d.Info()
	}
	return d.source.Stat()
}

func (d *callbackDirEntry) Depth() int { return d.source.Depth() }

func NewDirEntry(entry *WalkEntry) DirEntry {
	if entry == nil {
		return nil
	}
	clone := *entry
	clone.cb.source = &clone
	return &clone.cb
}

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
	names := [...]string{"canonicalize", "read directory", "read entry", "read metadata", "schedule worker"}
	if int(o) < len(names) {
		return names[o]
	}
	return "walk"
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
	WalkTraverseLink
)

var ErrTraverseLink = errors.New("treestamp: traverse symlink target directory")
var ErrSkipFiles = errors.New("skip remaining files in this directory")
var ErrSkipThis = errors.New("skip this directory entry")

type CallbackOptions struct {
	After            fs.WalkDirFunc
	ToSlash          bool
	ContentsFirst    bool
	Sort             bool
	Follow           bool
	Scratch          []byte
	RequireDirectory bool
	MaxOpen          int
	Context          context.Context
	FollowOutside    bool
}

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

func WalkCallback(root string, fn fs.WalkDirFunc, toSlash bool) error {
	return WalkCallbackHooks(root, fn, CallbackOptions{ToSlash: toSlash})
}

func WalkCallbackHooks(root string, fn fs.WalkDirFunc, opts CallbackOptions) error {
	return listwalk.Walk(root, fn, listwalk.Config{
		After: opts.After, ToSlash: opts.ToSlash, ContentsFirst: opts.ContentsFirst,
		Sort: opts.Sort, Follow: opts.Follow, Scratch: opts.Scratch,
		RequireDirectory: opts.RequireDirectory, MaxOpen: opts.MaxOpen, Context: opts.Context,
		SkipFiles: ErrSkipFiles, TraverseLink: ErrTraverseLink, SkipThis: ErrSkipThis,
		OnLink: func(path, name string, depth int, ancestors []string) (bool, error) {
			return traverseListed(root, path, name, depth, ancestors, opts.FollowOutside)
		},
	})
}

func traverseListed(root, path, name string, depth int, ancestors []string, outside bool) (bool, error) {
	walkEntry := &WalkEntry{root: root, path: path, name: name, depth: depth, symlink: true}
	opts := DefaultOptions()
	opts.FollowOutside = outside
	result, err := inspectSelectedLink(selectedLinkPolicy{
		root: root, path: path, depth: depth, entry: walkEntry,
		options: opts,
		ancestor: func(id platform.Identity) (bool, *WalkError) {
			return ancestorHasID(ancestors, id, depth)
		},
	})
	if err != nil {
		return false, err
	}
	return result.skip == SkipNone, nil
}

func ancestorHasID(paths []string, id platform.Identity, depth int) (bool, *WalkError) {
	for _, path := range paths {
		info, err := platform.DirectoryInfo(path)
		if err != nil {
			return false, walkErr(path, depth, OpReadMetadata, err)
		}
		if info.Identity == id {
			return true, nil
		}
	}
	return false, nil
}

func (w *callbackWork) follow(entry *listwalk.Entry, job dirJob) bool {
	walkEntry := &WalkEntry{root: w.root, path: entry.Path(), name: entry.Name(), depth: entry.Depth(), symlink: true}
	opts := DefaultOptions()
	opts.FollowOutside = w.followOutside
	result, err := inspectSelectedLink(selectedLinkPolicy{
		root: w.root, path: entry.Path(), depth: entry.Depth(), entry: walkEntry,
		options: opts,
		ancestor: func(id platform.Identity) (bool, *WalkError) {
			return chainHasID(w.root, job.path, id, entry.Depth())
		},
	})
	if err != nil {
		w.stop(err)
		return true
	}
	if result.skip == SkipNone {
		w.queue.push(dirJob{path: entry.Path(), depth: entry.Depth()})
	}
	return false
}

func showPath(path string, toSlash bool) string { return listwalk.Show(path, toSlash) }

func containsID(ids []platformID, id platform.Identity) bool {
	for _, item := range ids {
		if item.fs == id.FileSystem && item.file == id.File {
			return true
		}
	}
	return false
}

func childPath(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + string(os.PathSeparator) + name
}

func sortDirents(dents []os.DirEntry, mode int) {
	if mode == LocalSortNone || len(dents) < 2 {
		return
	}
	sort.SliceStable(dents, func(i, j int) bool {
		if mode != LocalSortLexical {
			ri, rj := direntRank(dents[i], mode), direntRank(dents[j], mode)
			if ri != rj {
				return ri < rj
			}
		}
		return dents[i].Name() < dents[j].Name()
	})
}

func direntRank(d os.DirEntry, mode int) int {
	dir, file := 2, 0
	if mode == LocalSortDirsFirst {
		dir, file = 0, 1
	}
	if d.IsDir() {
		return dir
	}
	if d.Type().IsRegular() {
		return file
	}
	return 1
}

func chainHasID(root, dir string, id platform.Identity, depth int) (bool, *WalkError) {
	for dir != "" {
		info, err := platform.DirectoryInfo(dir)
		if err != nil {
			return false, walkErr(dir, depth, OpReadMetadata, err)
		}
		if info.Identity == id {
			return true, nil
		}
		next := filepath.Dir(dir)
		if filepath.Clean(dir) == filepath.Clean(root) || next == dir {
			return false, nil
		}
		dir = next
	}
	return false, nil
}

func (b *StatefulWalkBuilder[R, E]) BuildParallelOrdered(capacity int) (*ParallelStatefulWalker[E], error) {
	if _, err := filepath.Abs(b.root); err != nil {
		return nil, walkErr(b.root, 0, OpCanonicalize, err)
	}
	iter := NewParallelWalker(b.root).Options(b.options).WithParallelism(b.workers).IntoIterOrderedBounded(capacity)
	process, state := b.process, b.rootState
	return &ParallelStatefulWalker[E]{next: func() (*StatefulWalkEntry[E], error) {
		entry, err := iter.Next()
		if err != nil {
			return nil, err
		}
		item := &StatefulWalkEntry[E]{Entry: entry, ReadChildren: entry.isDir && !entry.hasSkip()}
		if process != nil {
			batch := []StatefulResult[E]{{Entry: item}}
			st := state
			process(entry.depth, filepath.Dir(entry.path), &st, &batch)
			if len(batch) > 0 && batch[0].Entry != nil {
				item = batch[0].Entry
			}
		}
		return item, nil
	}, close: iter.Close}, nil
}

func listedBytes(l *listedDir) int64 {
	if l == nil || l.err != nil {
		return 0
	}
	n := int64(len(l.entries)+len(l.descents)) * 64
	for _, e := range l.entries {
		n += int64(len(e.path) + len(e.name))
	}
	return n
}

func sendOrdered(quit <-chan struct{}, ch chan item, it item) bool {
	select {
	case <-quit:
		return false
	case ch <- it:
		return true
	}
}
