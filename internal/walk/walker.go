package walk

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/sergii-ziborov/treestamp/internal/dirread"
	"github.com/sergii-ziborov/treestamp/internal/listwalk"
	pathx "github.com/sergii-ziborov/treestamp/internal/path"
	"github.com/sergii-ziborov/treestamp/internal/platform"
)

type Walker struct {
	root                                                                                                string
	rootBytes                                                                                           *uint64
	rootVersion                                                                                         *FileVersion
	rootFS                                                                                              *uint64
	rootInfo                                                                                            *platform.Info
	options                                                                                             WalkOptions
	frames                                                                                              []dirFrame
	openHandles                                                                                         int
	pending                                                                                             *pendingDir
	current                                                                                             *WalkEntry
	active                                                                                              map[platform.Identity]int
	sorter                                                                                              func(a, b os.DirEntry) int
	filter                                                                                              func(*WalkEntry) bool
	skipStdout                                                                                          *platform.Identity
	deferred                                                                                            *WalkEntry
	rootMeta                                                                                            os.FileInfo
	rootIsDir, rootIsFile, rootIsSymlink, yieldRoot, skipPending, finished, contentsFirst, plainEntries bool
}

type dirFrame struct {
	path      string
	depth     int
	entries   dirEntries
	identity  *platform.Identity
	postEntry *WalkEntry
}

type walkerConfig struct {
	sorter        func(a, b os.DirEntry) int
	filter        func(*WalkEntry) bool
	skipStdout    *platform.Identity
	contentsFirst bool
}

func New(root string) (*Walker, error) {
	return NewWithOptions(root, DefaultOptions())
}

func NewWithOptions(root string, options WalkOptions) (*Walker, error) {
	return newWalker(root, options, walkerConfig{})
}

func newWalker(root string, options WalkOptions, cfg walkerConfig) (*Walker, error) {
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
	canonical, err := resolveWalkRoot(root, options)
	if err != nil {
		return nil, err
	}
	return finishWalker(canonical, options, cfg, info)
}

func resolveWalkRoot(root string, options WalkOptions) (string, error) {
	if options.FollowLinks || options.SameFileSystem {
		abs, absErr := filepath.Abs(root)
		if absErr != nil {
			return "", walkErr(root, 0, OpCanonicalize, absErr)
		}
		resolved, resErr := pathx.Resolve(abs)
		if resErr != nil {
			return "", walkErr(root, 0, OpCanonicalize, resErr)
		}
		return resolved, nil
	}
	if !filepath.IsAbs(root) {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return "", walkErr(root, 0, OpCanonicalize, cwdErr)
		}
		return filepath.Join(cwd, root), nil
	}
	return root, nil
}

func finishWalker(canonical string, options WalkOptions, cfg walkerConfig, linkInfo os.FileInfo) (*Walker, error) {
	meta, err := os.Stat(canonical)
	if err != nil {
		return nil, walkErr(canonical, 0, OpReadMetadata, err)
	}
	rootInfo, rootFS, err := rootPlatform(canonical, meta, options)
	if err != nil {
		return nil, err
	}
	rootBytes, rootVersion := rootFileMeta(canonical, meta, options)
	plain := !options.FollowLinks && !options.SameFileSystem && !options.CollectMetadata &&
		options.MaxDepth == nil && options.MinDepth == 0 &&
		cfg.filter == nil && cfg.skipStdout == nil && !cfg.contentsFirst
	return &Walker{
		root: canonical, rootIsDir: meta.IsDir(), rootIsFile: meta.Mode().IsRegular(),
		rootIsSymlink: meta.Mode()&os.ModeSymlink != 0, rootBytes: rootBytes, rootVersion: rootVersion,
		rootFS: rootFS, rootInfo: rootInfo, options: options, yieldRoot: true,
		active: make(map[platform.Identity]int), sorter: cfg.sorter, filter: cfg.filter,
		skipStdout: cfg.skipStdout, contentsFirst: cfg.contentsFirst, plainEntries: plain, rootMeta: linkInfo,
	}, nil
}

func rootPlatform(canonical string, meta os.FileInfo, options WalkOptions) (*platform.Info, *uint64, error) {
	if !meta.IsDir() || (!options.FollowLinks && !options.SameFileSystem) {
		return nil, nil, nil
	}
	infoVal, infoErr := platform.DirectoryInfo(canonical)
	if infoErr != nil {
		return nil, nil, walkErr(canonical, 0, OpReadMetadata, infoErr)
	}
	var rootFS *uint64
	if options.SameFileSystem {
		fsid := infoVal.FileSystem
		rootFS = &fsid
	}
	return &infoVal, rootFS, nil
}

func rootFileMeta(canonical string, meta os.FileInfo, options WalkOptions) (*uint64, *FileVersion) {
	if !options.CollectMetadata || !meta.Mode().IsRegular() {
		return nil, nil
	}
	size := uint64(meta.Size())
	ver := versionFromInfo(meta)
	return &size, &ver
}

var (
	errRootSymlink      = errText("root symlink rejected by policy")
	errNoCurrentSymlink = errText("current walk entry is not a symlink")
)

type errText string

func (e errText) Error() string { return string(e) }
func (w *Walker) Root() string         { return w.root }
func (w *Walker) Options() WalkOptions { return w.options }

func (w *Walker) SkipCurrentDir() {
	if w.pending != nil {
		w.skipPending = true
	}
}

func (w *Walker) TraverseCurrentSymlink() error {
	entry := w.current
	if entry == nil {
		return walkErr(w.root, 0, OpReadMetadata, errNoCurrentSymlink)
	}
	if !entry.symlink {
		return walkErr(entry.path, entry.depth, OpReadMetadata, errNoCurrentSymlink)
	}
	if entry.hasSkip() || (w.pending != nil && w.pending.path == entry.path) {
		return nil
	}
	if followErr := w.prepareSelectedLink(entry); followErr != nil {
		return followErr
	}
	return nil
}

func (w *Walker) Close() error {
	w.finished = true
	for i := range w.frames {
		w.frames[i].entries.close()
	}
	w.frames = nil
	w.openHandles = 0
	w.pending = nil
	w.current = nil
	w.active = nil
	return nil
}

func (w *Walker) Next() (*WalkEntry, error) {
	w.current = nil
	for {
		if w.finished {
			return nil, io.EOF
		}
		if w.deferred != nil {
			entry := w.deferred
			w.deferred = nil
			return w.remember(entry), nil
		}
		if w.yieldRoot {
			w.yieldRoot = false
			entry, err := w.takeRoot()
			if err != nil {
				return w.yieldError(err)
			}
			if entry != nil {
				return w.remember(entry), nil
			}
			continue
		}
		if err := w.schedulePending(); err != nil {
			return w.yieldError(err)
		}
		entry, err, cont := w.nextFromFrame()
		if cont {
			continue
		}
		return w.remember(entry), err
	}
}

func (w *Walker) remember(entry *WalkEntry) *WalkEntry {
	w.current = entry
	return entry
}
func (w *Walker) nextFromFrame() (*WalkEntry, error, bool) {
	if len(w.frames) == 0 {
		w.finished = true
		return nil, io.EOF, false
	}
	frame := &w.frames[len(w.frames)-1]
	depth := frame.depth + 1
	dirent, readErr, ok := frame.entries.next()
	if !ok {
		return w.popFrame(frame)
	}
	if readErr != nil {
		entry, err := w.yieldError(walkErr(frame.path, depth, OpReadEntry, readErr))
		return entry, err, false
	}
	return w.visitDirent(frame.path, dirent, depth)
}

func (w *Walker) popFrame(frame *dirFrame) (*WalkEntry, error, bool) {
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
		return post, nil, false
	}
	return nil, nil, true
}

func (w *Walker) visitDirent(dir string, dirent os.DirEntry, depth int) (*WalkEntry, error, bool) {
	path := childPath(dir, dirent.Name())
	mode, typeErr := entryMode(dirent)
	if typeErr != nil {
		entry, err := w.yieldError(walkErr(path, depth, OpReadMetadata, typeErr))
		return entry, err, false
	}
	if w.plainEntries {
		return w.visitPlain(path, depth, dirent, nil), nil, false
	}
	bytes, version, hidden, err := w.direntMeta(path, dirent, mode)
	if err != nil {
		entry, yieldErr := w.yieldError(err)
		return entry, yieldErr, false
	}
	entry, visitErr := w.visit(path, depth, mode, bytes, version, hidden)
	if visitErr != nil {
		out, yieldErr := w.yieldError(visitErr)
		return out, yieldErr, false
	}
	entry.dent = dirent
	prepared := w.prepare(entry)
	if prepared == nil {
		return nil, nil, true
	}
	return prepared, nil, false
}

func (w *Walker) direntMeta(path string, dirent os.DirEntry, mode os.FileMode) (*uint64, *FileVersion, *bool, *WalkError) {
	if !w.options.CollectMetadata || !mode.IsRegular() || mode&os.ModeSymlink != 0 {
		return nil, nil, nil, nil
	}
	info, infoErr := dirent.Info()
	if infoErr != nil {
		return nil, nil, nil, walkErr(path, 0, OpReadMetadata, infoErr)
	}
	size := uint64(info.Size())
	ver := versionFromInfo(info)
	h := platform.HiddenFromInfo(path, info)
	return &size, &ver, &h, nil
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
		entry := w.visitPlain(w.root, 0, nil, w.rootMeta)
		if w.rootIsDir && w.pending == nil {
			w.pending = &pendingDir{path: w.root, depth: 0}
		}
		return entry, nil
	}
	entry, err := w.visit(w.root, 0, mode, w.rootBytes, w.rootVersion, nil)
	if err != nil {
		return nil, err
	}
	entry.info = w.rootMeta
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
	return w.openPending(pending)
}

func (w *Walker) openPending(pending *pendingDir) *WalkError {
	if w.sorter != nil {
		entries, err := collectSorted(pending.path, w.sorter)
		if err != nil {
			w.deferred = pending.postEntry
			return walkErr(pending.path, pending.depth, OpReadDirectory, err)
		}
		w.pushFrame(pending, entries)
		return nil
	}
	opened, err := openDirectory(pending.path)
	if err != nil {
		w.deferred = pending.postEntry
		return walkErr(pending.path, pending.depth, OpReadDirectory, err)
	}
	w.pushFrame(pending, opened)
	w.openHandles++
	return nil
}

func (w *Walker) pushFrame(pending *pendingDir, entries dirEntries) {
	if pending.identity != nil {
		w.active[*pending.identity]++
	}
	w.frames = append(w.frames, dirFrame{
		path: pending.path, depth: pending.depth, entries: entries,
		identity: pending.identity, postEntry: pending.postEntry,
	})
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

type callbackWork struct {
	root                                string
	fn                                  fs.WalkDirFunc
	toSlash, keepSkipAll, followOutside bool
	localSort                           int
	queue                               *dirQueue
	mu                                  sync.Mutex
	err                                 error
	quit                                atomic.Bool
}
func WalkCallbackParallelOpts(root string, fn fs.WalkDirFunc, opts ParallelCallback) error {
	abs, descend, err := prepareCallback(root, fn, opts)
	if err != nil || !descend {
		return err
	}
	work := &callbackWork{
		root: abs, fn: fn, toSlash: opts.ToSlash, keepSkipAll: opts.KeepSkipAll,
		followOutside: opts.FollowOutside, localSort: opts.LocalSort, queue: newDirQueue(),
	}
	work.visitDir(dirJob{path: abs, depth: 0})
	return work.finish(opts.Workers)
}

func (w *callbackWork) finish(workers int) error {
	jobs := w.queue.takeAll()
	n := parallelWorkers(workers)
	if n > 1 && len(jobs) > 1 {
		for _, job := range jobs {
			w.queue.push(job)
		}
		var wg sync.WaitGroup
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() { defer wg.Done(); w.run() }()
		}
		wg.Wait()
		return w.err
	}
	for len(jobs) > 0 && !w.quit.Load() {
		for _, job := range jobs {
			w.visitDir(job)
		}
		jobs = w.queue.takeAll()
	}
	return w.err
}
func parallelWorkers(n int) int {
	if n <= 0 {
		n = runtime.GOMAXPROCS(0)
	}
	if n < 1 {
		return 1
	}
	if n > 8 {
		return 8
	}
	return n
}

func prepareCallback(root string, fn fs.WalkDirFunc, opts ParallelCallback) (string, bool, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", false, fn(root, nil, err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", false, fn(showPath(abs, opts.ToSlash), nil, err)
	}
	entry := listwalk.Own(info.Name(), abs, info.Mode().Type(), 0, info)
	cbErr := listwalk.Deliver(fn, entry, opts.ToSlash)
	typ := entry.Type()
	if cbErr != nil {
		if errors.Is(cbErr, fs.SkipAll) {
			cbErr = keepSkipAllErr(opts.KeepSkipAll)
		} else if errors.Is(cbErr, fs.SkipDir) || errors.Is(cbErr, ErrSkipThis) {
			cbErr = nil
		}
		return "", false, cbErr
	}
	if typ.IsDir() {
		return abs, true, nil
	}
	if typ&os.ModeSymlink == 0 {
		return abs, false, nil
	}
	target, err := os.Stat(abs)
	return abs, err == nil && target.IsDir(), nil
}

func (w *callbackWork) run() {
	for job, ok := w.queue.pop(); ok; job, ok = w.queue.pop() {
		w.visitDir(job)
		w.queue.done()
	}
}

func (w *callbackWork) visitDir(job dirJob) {
	if w.quit.Load() {
		return
	}
	if w.localSort == LocalSortNone {
		w.streamDir(job)
		return
	}
	dents, err := dirread.OSEntries(job.path)
	if err != nil {
		w.reportRead(job.path, err)
		return
	}
	sortDirents(dents, w.localSort)
	skipFiles := false
	for i := range dents {
		if w.quit.Load() {
			return
		}
		if skipFiles && dents[i].Type().IsRegular() {
			continue
		}
		entry := listwalk.Own(dents[i].Name(), childPath(job.path, dents[i].Name()), dents[i].Type(), job.depth+1, nil)
		entry.Bind(dents[i])
		if w.control(listwalk.Deliver(w.fn, entry, w.toSlash), entry, job, &skipFiles) {
			return
		}
	}
}

func (w *callbackWork) control(err error, entry *listwalk.Entry, job dirJob, skipFiles *bool) bool {
	switch {
	case err == nil:
		if entry.IsDir() {
			w.queue.push(dirJob{path: entry.Path(), depth: entry.Depth()})
		}
		return false
	case errors.Is(err, fs.SkipDir):
		return !entry.IsDir()
	case errors.Is(err, fs.SkipAll):
		w.stop(keepSkipAllErr(w.keepSkipAll))
		return true
	case errors.Is(err, ErrSkipFiles):
		*skipFiles = true
		return false
	case errors.Is(err, ErrTraverseLink) && entry.Type()&os.ModeSymlink != 0:
		return w.follow(entry, job)
	default:
		w.stop(err)
		return true
	}
}

func (w *callbackWork) reportRead(path string, err error) {
	cbErr := w.fn(showPath(path, w.toSlash), nil, err)
	if cbErr == nil || errors.Is(cbErr, fs.SkipDir) {
		return
	}
	if errors.Is(cbErr, fs.SkipAll) {
		cbErr = keepSkipAllErr(w.keepSkipAll)
	}
	w.stop(cbErr)
}

func keepSkipAllErr(keep bool) error { if keep { return fs.SkipAll }; return nil }

func (w *callbackWork) stop(err error) {
	w.mu.Lock()
	if w.err == nil {
		w.err = err
	}
	w.mu.Unlock()
	w.quit.Store(true)
	w.queue.close()
}
