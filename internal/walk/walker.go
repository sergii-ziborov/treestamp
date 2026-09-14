package walk

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sergii-ziborov/treestamp/internal/platform"
)

// Walker is an iterative depth-first filesystem walker.
// It does not call filepath.WalkDir.
type Walker struct {
	root          string
	rootIsDir     bool
	rootIsFile    bool
	rootIsSymlink bool
	rootBytes     *uint64
	rootVersion   *FileVersion
	rootFS        *uint64
	rootInfo      *platform.Info
	options       WalkOptions
	frames        []dirFrame
	openHandles   int
	yieldRoot     bool
	pending       *pendingDir
	skipPending   bool
	active        map[platform.Identity]int
	finished      bool
	sorter        func(a, b os.DirEntry) int
	filter        func(*WalkEntry) bool
	skipStdout    *platform.Identity
	contentsFirst bool
	deferred      *WalkEntry
	plainEntries  bool
}

type dirFrame struct {
	path      string
	depth     int
	entries   dirEntries
	identity  *platform.Identity
	postEntry *WalkEntry
}

// New creates a walker with default options.
func New(root string) (*Walker, error) {
	return NewWithOptions(root, DefaultOptions())
}

// NewWithOptions creates a walker with an explicit policy.
func NewWithOptions(root string, options WalkOptions) (*Walker, error) {
	return newWalker(root, options, nil, nil, nil, false)
}

func newWalker(
	root string,
	options WalkOptions,
	sorter func(a, b os.DirEntry) int,
	filter func(*WalkEntry) bool,
	skipStdout *platform.Identity,
	contentsFirst bool,
) (*Walker, error) {
	if root == "" {
		return nil, walkErr(root, 0, OpCanonicalize, os.ErrInvalid)
	}
	options = options.Normalize()
	info, err := os.Lstat(root)
	if err != nil {
		return nil, walkErr(root, 0, OpReadMetadata, err)
	}
	if options.RootSymlinkPolicy == RootReject && info.Mode()&os.ModeSymlink != 0 {
		return nil, walkErr(root, 0, OpReadMetadata, errRootSymlink)
	}

	canonical := root
	if options.FollowLinks || options.SameFileSystem {
		abs, absErr := filepath.Abs(root)
		if absErr != nil {
			return nil, walkErr(root, 0, OpCanonicalize, absErr)
		}
		resolved, resErr := filepath.EvalSymlinks(abs)
		if resErr != nil {
			return nil, walkErr(root, 0, OpCanonicalize, resErr)
		}
		canonical = resolved
	} else if !filepath.IsAbs(root) {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return nil, walkErr(root, 0, OpCanonicalize, cwdErr)
		}
		canonical = filepath.Join(cwd, root)
	}

	meta, err := os.Stat(canonical)
	if err != nil {
		return nil, walkErr(canonical, 0, OpReadMetadata, err)
	}

	var rootInfo *platform.Info
	var rootFS *uint64
	if meta.IsDir() && (options.FollowLinks || options.SameFileSystem) {
		infoVal, infoErr := platform.DirectoryInfo(canonical)
		if infoErr != nil {
			return nil, walkErr(canonical, 0, OpReadMetadata, infoErr)
		}
		rootInfo = &infoVal
		if options.SameFileSystem {
			fsid := infoVal.FileSystem
			rootFS = &fsid
		}
	}

	var rootBytes *uint64
	var rootVersion *FileVersion
	if options.CollectMetadata && meta.Mode().IsRegular() {
		size := uint64(meta.Size())
		rootBytes = &size
		ver := versionFromInfo(canonical, meta)
		rootVersion = &ver
	}

	plain := !options.FollowLinks &&
		!options.SameFileSystem &&
		!options.CollectMetadata &&
		options.MaxDepth == nil &&
		options.MinDepth == 0 &&
		filter == nil &&
		skipStdout == nil &&
		!contentsFirst

	return &Walker{
		root:          canonical,
		rootIsDir:     meta.IsDir(),
		rootIsFile:    meta.Mode().IsRegular(),
		rootIsSymlink: meta.Mode()&os.ModeSymlink != 0,
		rootBytes:     rootBytes,
		rootVersion:   rootVersion,
		rootFS:        rootFS,
		rootInfo:      rootInfo,
		options:       options,
		yieldRoot:     true,
		active:        make(map[platform.Identity]int),
		sorter:        sorter,
		filter:        filter,
		skipStdout:    skipStdout,
		contentsFirst: contentsFirst,
		plainEntries:  plain,
	}, nil
}

var errRootSymlink = errText("root symlink rejected by policy")

type errText string

func (e errText) Error() string { return string(e) }

// Root returns the resolved walk root.
func (w *Walker) Root() string { return w.root }

// Options returns the normalized policy.
func (w *Walker) Options() WalkOptions { return w.options }

// SkipCurrentDir prevents descent into the directory returned by the previous Next.
func (w *Walker) SkipCurrentDir() {
	if w.pending != nil {
		w.skipPending = true
	}
}

// Close releases open directory handles.
func (w *Walker) Close() error {
	w.finished = true
	for i := range w.frames {
		w.frames[i].entries.close()
	}
	w.frames = nil
	w.openHandles = 0
	w.pending = nil
	w.active = nil
	return nil
}

// Next returns the next entry. io.EOF ends the walk. A WalkError is a local
// failure; ErrorContinue keeps going.
func (w *Walker) Next() (*WalkEntry, error) {
	for {
		if w.finished {
			return nil, io.EOF
		}
		if w.deferred != nil {
			entry := w.deferred
			w.deferred = nil
			return entry, nil
		}
		if w.yieldRoot {
			w.yieldRoot = false
			entry, err := w.takeRoot()
			if err != nil {
				return w.yieldError(err)
			}
			if entry != nil {
				return entry, nil
			}
			continue
		}
		if err := w.schedulePending(); err != nil {
			return w.yieldError(err)
		}
		if len(w.frames) == 0 {
			w.finished = true
			return nil, io.EOF
		}
		frame := &w.frames[len(w.frames)-1]
		depth := frame.depth + 1
		dirent, readErr, ok := frame.entries.next()
		if !ok {
			wasOpen := frame.entries.isOpen()
			identity := frame.identity
			post := frame.postEntry
			frame.entries.close()
			w.frames = w.frames[:len(w.frames)-1]
			if wasOpen {
				w.openHandles--
			}
			if identity != nil {
				if n := w.active[*identity] - 1; n <= 0 {
					delete(w.active, *identity)
				} else {
					w.active[*identity] = n
				}
			}
			if post != nil {
				return post, nil
			}
			continue
		}
		if readErr != nil {
			return w.yieldError(walkErr(frame.path, depth, OpReadEntry, readErr))
		}
		path := childPath(frame.path, dirent.Name())
		mode, typeErr := entryMode(dirent)
		if typeErr != nil {
			return w.yieldError(walkErr(path, depth, OpReadMetadata, typeErr))
		}
		if w.plainEntries {
			return w.visitPlain(path, depth, mode), nil
		}
		var bytes *uint64
		var version *FileVersion
		var hidden *bool
		if w.options.CollectMetadata && mode.IsRegular() && mode&os.ModeSymlink == 0 {
			info, infoErr := dirent.Info()
			if infoErr != nil {
				return w.yieldError(walkErr(path, depth, OpReadMetadata, infoErr))
			}
			size := uint64(info.Size())
			bytes = &size
			ver := versionFromInfo(path, info)
			version = &ver
			h := platform.HiddenFromInfo(path, info)
			hidden = &h
		}
		entry, err := w.visit(path, depth, mode, bytes, version, hidden)
		if err != nil {
			return w.yieldError(err)
		}
		prepared := w.prepare(entry)
		if prepared == nil {
			continue
		}
		return prepared, nil
	}
}

func (w *Walker) takeRoot() (*WalkEntry, *WalkError) {
	mode := os.ModeDir
	if w.rootIsFile {
		mode = 0
	}
	if w.rootIsSymlink {
		mode |= os.ModeSymlink
	}
	if w.plainEntries {
		return w.visitPlain(w.root, 0, mode), nil
	}
	entry, err := w.visit(w.root, 0, mode, w.rootBytes, w.rootVersion, nil)
	if err != nil {
		return nil, err
	}
	return w.prepare(entry), nil
}

func (w *Walker) prepare(entry *WalkEntry) *WalkEntry {
	if entry.isFile && w.skipStdout != nil {
		match, err := platform.PathMatchesIdentity(entry.path, *w.skipStdout)
		if err == nil && match {
			return nil
		}
	}
	if w.filter != nil && !w.filter(entry) {
		if entry.isDir {
			w.SkipCurrentDir()
			if entry.depth == 0 {
				w.finished = true
			}
		}
		return nil
	}
	if w.contentsFirst && entry.isDir && !entry.hasSkip() {
		if entry.depth >= w.options.MinDepth && w.pending != nil {
			w.pending.postEntry = entry
		}
		return nil
	}
	if entry.depth >= w.options.MinDepth {
		return entry
	}
	return nil
}

func (w *Walker) schedulePending() *WalkError {
	if w.pending == nil {
		return nil
	}
	pending := w.pending
	w.pending = nil
	if w.skipPending {
		w.skipPending = false
		return nil
	}
	if w.openHandles >= w.options.MaxOpen {
		w.bufferOldest()
	}
	if w.sorter != nil {
		entries, err := collectSorted(pending.path, w.sorter)
		if err != nil {
			w.deferred = pending.postEntry
			return walkErr(pending.path, pending.depth, OpReadDirectory, err)
		}
		w.frames = append(w.frames, dirFrame{
			path:      pending.path,
			depth:     pending.depth,
			entries:   entries,
			identity:  pending.identity,
			postEntry: pending.postEntry,
		})
		if pending.identity != nil {
			w.active[*pending.identity]++
		}
		return nil
	}
	opened, err := openDirectory(pending.path)
	if err != nil {
		w.deferred = pending.postEntry
		return walkErr(pending.path, pending.depth, OpReadDirectory, err)
	}
	if pending.identity != nil {
		w.active[*pending.identity]++
	}
	w.frames = append(w.frames, dirFrame{
		path:      pending.path,
		depth:     pending.depth,
		entries:   opened,
		identity:  pending.identity,
		postEntry: pending.postEntry,
	})
	w.openHandles++
	return nil
}

func (w *Walker) bufferOldest() {
	for i := range w.frames {
		if w.frames[i].entries.isOpen() {
			w.frames[i].entries.drain()
			w.openHandles--
			return
		}
	}
}

func (w *Walker) yieldError(err *WalkError) (*WalkEntry, error) {
	if w.options.ErrorPolicy == ErrorAbort {
		_ = w.Close()
	}
	return nil, err
}

func entryMode(entry os.DirEntry) (os.FileMode, error) {
	mode := entry.Type()
	if mode != 0 {
		return mode, nil
	}
	info, err := entry.Info()
	if err != nil {
		return 0, err
	}
	return info.Mode(), nil
}

func versionFromInfo(path string, info os.FileInfo) FileVersion {
	var ver FileVersion
	if !info.ModTime().IsZero() {
		ns := uint64(info.ModTime().UnixNano())
		if info.ModTime().After(time.Unix(0, 0)) {
			ver.ModifiedNS = &ns
		}
	}
	if id, err := platform.PathIdentity(path); err == nil {
		ver.Identity = &id
	}
	_ = path
	return ver
}
