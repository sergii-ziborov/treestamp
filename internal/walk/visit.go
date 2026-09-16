package walk

import (
	"io"
	"os"
	"path/filepath"

	pathx "github.com/sergii-ziborov/treestamp/internal/path"
	"github.com/sergii-ziborov/treestamp/internal/platform"
)

const readDirBatch = 64

type pendingDir struct {
	path      string
	depth     int
	identity  *platform.Identity
	postEntry *WalkEntry
}

type dirEntries interface {
	next() (os.DirEntry, error, bool)
	isOpen() bool
	close()
	drain()
}

func (w *Walker) visitPlain(path string, depth int, dent os.DirEntry, info os.FileInfo) *WalkEntry {
	mode := fileModeOf(dent, info)
	symlink := mode&os.ModeSymlink != 0
	isFile := !symlink && mode.IsRegular()
	isDir := !symlink && mode.IsDir()
	if isDir {
		w.pending = &pendingDir{path: path, depth: depth}
	}
	return &WalkEntry{
		root: w.root, path: path, name: entryName(dent, info), depth: depth,
		isFile: isFile, isDir: isDir, symlink: symlink, dent: dent, info: info,
	}
}

func fileModeOf(dent os.DirEntry, info os.FileInfo) os.FileMode {
	if dent != nil {
		if typ := dent.Type(); typ != 0 {
			return typ
		}
	}
	if info != nil {
		return info.Mode()
	}
	return 0
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

func (s *concurrentState) needsEntryInfo() bool {
	opts := s.opts.options
	return opts.FollowLinks || opts.SameFileSystem || opts.CollectMetadata
}

func (s *concurrentState) entryFromDent(job dirJob, dent os.DirEntry, path string) (*WalkEntry, *WalkError) {
	if !s.needsEntryInfo() {
		mode, modeErr := entryMode(dent)
		if modeErr != nil {
			return nil, walkErr(path, job.depth+1, OpReadMetadata, modeErr)
		}
		return plainWalkEntry(s.abs, path, job.depth+1, dent, mode), nil
	}
	info, infoErr := dent.Info()
	if infoErr != nil {
		return nil, walkErr(path, job.depth+1, OpReadMetadata, infoErr)
	}
	if info.Mode()&os.ModeSymlink != 0 && s.opts.options.FollowLinks {
		target, err := os.Stat(path)
		if err != nil {
			return nil, walkErr(path, job.depth+1, OpReadMetadata, err)
		}
		entry := makeEntry(s.abs, path, job.depth+1, info, s.opts.options, target)
		entry.dent = dent
		return entry, nil
	}
	entry := makeEntry(s.abs, path, job.depth+1, info, s.opts.options, nil)
	entry.dent = dent
	return entry, nil
}

func plainWalkEntry(root, path string, depth int, dent os.DirEntry, mode os.FileMode) *WalkEntry {
	symlink := mode&os.ModeSymlink != 0
	return &WalkEntry{
		root: root, path: path, name: dent.Name(), depth: depth,
		isFile: !symlink && mode.IsRegular(), isDir: !symlink && mode.IsDir(),
		symlink: symlink, dent: dent,
	}
}

func entryName(dent os.DirEntry, info os.FileInfo) string {
	if dent != nil {
		return dent.Name()
	}
	if info != nil {
		return info.Name()
	}
	return ""
}

func (w *Walker) visit(path string, depth int, mode os.FileMode, bytes *uint64, version *FileVersion, hidden *bool) (*WalkEntry, *WalkError) {
	symlink := mode&os.ModeSymlink != 0
	isFile := mode.IsRegular()
	isDir := mode.IsDir()
	target, meta, err := w.followTarget(path, depth, symlink, isFile, visitMeta{bytes: bytes, version: version, hidden: hidden})
	if err != nil {
		return nil, err
	}
	bytes, version, hidden = meta.bytes, meta.version, meta.hidden
	var skip WalkSkipReason
	var dirID *platform.Identity
	if symlink && w.options.FollowLinks && target != nil {
		isFile = target.Mode().IsRegular()
		isDir = target.IsDir()
	}
	if symlink && !w.options.FollowLinks {
		isFile = false
		isDir = false
	} else {
		skip, dirID, err = w.classifyDir(path, depth, symlink, isDir, target)
		if err != nil {
			return nil, err
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
	stat := newFileInfoCache()
	if target != nil {
		stat.load(target, nil)
	}
	return &WalkEntry{
		root: w.root, path: path, depth: depth, isFile: isFile, isDir: isDir,
		symlink: symlink, bytes: bytes, version: version, hidden: hidden, dirID: dirID, skip: skip, stat: stat,
	}, nil
}

type visitMeta struct {
	bytes   *uint64
	version *FileVersion
	hidden  *bool
}

func (w *Walker) followTarget(path string, depth int, symlink, isFile bool, meta visitMeta) (os.FileInfo, visitMeta, *WalkError) {
	if !symlink || !w.options.FollowLinks {
		return nil, meta, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, visitMeta{}, walkErr(path, depth, OpReadMetadata, err)
	}
	if w.options.CollectMetadata && info.Mode().IsRegular() {
		size := uint64(info.Size())
		meta.bytes = &size
		ver := versionFromInfo(path, info)
		meta.version = &ver
		h := platform.HiddenFromInfo(path, info)
		meta.hidden = &h
	}
	_ = isFile
	return info, meta, nil
}

func (w *Walker) classifyDir(path string, depth int, symlink, isDir bool, target os.FileInfo) (WalkSkipReason, *platform.Identity, *WalkError) {
	need := symlink || (isDir && (w.options.SameFileSystem || w.options.FollowLinks))
	if !need {
		return SkipNone, nil, nil
	}
	canonical := w.root
	if depth != 0 {
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			return SkipNone, nil, walkErr(path, depth, OpCanonicalize, absErr)
		}
		resolved, resErr := filepath.EvalSymlinks(abs)
		if resErr != nil {
			return SkipNone, nil, walkErr(path, depth, OpCanonicalize, resErr)
		}
		canonical = resolved
	}
	if !pathx.UnderRoot(w.root, canonical) {
		return SkipPathEscape, nil, nil
	}
	if !isDir {
		return SkipNone, nil, nil
	}
	return w.dirIdentity(path, depth, canonical, target)
}

func (w *Walker) dirIdentity(path string, depth int, canonical string, target os.FileInfo) (WalkSkipReason, *platform.Identity, *WalkError) {
	var info platform.Info
	if depth == 0 && w.rootInfo != nil {
		info = *w.rootInfo
	} else {
		if target == nil {
			st, stErr := os.Stat(path)
			if stErr != nil {
				return SkipNone, nil, walkErr(path, depth, OpReadMetadata, stErr)
			}
			_ = st
		}
		got, infoErr := platform.DirectoryInfo(canonical)
		if infoErr != nil {
			return SkipNone, nil, walkErr(path, depth, OpReadMetadata, infoErr)
		}
		info = got
	}
	var skip WalkSkipReason
	if w.options.SameFileSystem && w.rootFS != nil && info.FileSystem != *w.rootFS {
		skip = SkipFileSystemBoundary
	}
	var dirID *platform.Identity
	if w.options.FollowLinks {
		id := info.Identity
		dirID = &id
	}
	return skip, dirID, nil
}

var errSelectedLinkNotDir = errText("selected symlink target is not a directory")

type selectedLinkPolicy struct {
	root     string
	path     string
	depth    int
	entry    *WalkEntry
	options  WalkOptions
	rootFS   *uint64
	ancestor func(platform.Identity) (bool, *WalkError)
}

type selectedLinkResult struct {
	identity *platform.Identity
	skip     WalkSkipReason
}

func inspectSelectedLink(p selectedLinkPolicy) (selectedLinkResult, *WalkError) {
	target, err := p.entry.Stat()
	if err != nil {
		return selectedLinkResult{}, walkErr(p.path, p.depth, OpReadMetadata, err)
	}
	if !target.IsDir() {
		return selectedLinkResult{}, walkErr(p.path, p.depth, OpReadDirectory, errSelectedLinkNotDir)
	}
	root, err := filepath.EvalSymlinks(p.root)
	if err != nil {
		return selectedLinkResult{}, walkErr(p.path, p.depth, OpCanonicalize, err)
	}
	resolved, err := filepath.EvalSymlinks(p.path)
	if err != nil {
		return selectedLinkResult{}, walkErr(p.path, p.depth, OpCanonicalize, err)
	}
	if !pathx.UnderRoot(root, resolved) {
		return selectedLinkResult{skip: SkipPathEscape}, nil
	}
	info, err := platform.DirectoryInfo(resolved)
	if err != nil {
		return selectedLinkResult{}, walkErr(p.path, p.depth, OpReadMetadata, err)
	}
	id := info.Identity
	result := selectedLinkResult{identity: &id}
	if p.options.SameFileSystem && p.rootFS != nil && info.FileSystem != *p.rootFS {
		result.skip = SkipFileSystemBoundary
	} else if p.options.atOrBeyondMaxDepth(p.depth) {
		result.skip = SkipMaxDepth
	} else if loop, loopErr := p.ancestor(id); loopErr != nil {
		return selectedLinkResult{}, loopErr
	} else if loop {
		result.skip = SkipSymlinkLoop
	}
	return result, nil
}

func applySelectedLink(entry *WalkEntry, result selectedLinkResult) {
	entry.isFile = false
	entry.isDir = true
	entry.dirID = result.identity
	entry.skip = result.skip
}

func (w *Walker) prepareSelectedLink(entry *WalkEntry) *WalkError {
	result, err := inspectSelectedLink(selectedLinkPolicy{
		root: w.root, path: entry.path, depth: entry.depth, entry: entry, options: w.options, rootFS: w.rootFS,
		ancestor: func(id platform.Identity) (bool, *WalkError) {
			return w.selectedAncestor(id, entry.depth)
		},
	})
	if err != nil {
		return err
	}
	applySelectedLink(entry, result)
	if result.skip == SkipNone {
		w.pending = &pendingDir{path: entry.path, depth: entry.depth, identity: result.identity}
	}
	return nil
}

func (w *Walker) selectedAncestor(id platform.Identity, depth int) (bool, *WalkError) {
	if w.active[id] > 0 {
		return true, nil
	}
	for i := range w.frames {
		info, err := platform.DirectoryInfo(w.frames[i].path)
		if err != nil {
			return false, walkErr(w.frames[i].path, depth, OpReadMetadata, err)
		}
		if info.Identity == id {
			return true, nil
		}
	}
	return false, nil
}

func (s *concurrentState) prepareSelectedLink(entry *WalkEntry, job dirJob) (*WalkEntry, *WalkError) {
	var rootFS *uint64
	if s.haveFS {
		fsid := s.rootFS
		rootFS = &fsid
	}
	result, err := inspectSelectedLink(selectedLinkPolicy{
		root: s.abs, path: entry.path, depth: entry.depth, entry: entry, options: s.opts.options, rootFS: rootFS,
		ancestor: func(id platform.Identity) (bool, *WalkError) {
			return s.selectedAncestor(job.path, id, entry.depth)
		},
	})
	if err != nil {
		return nil, err
	}
	selected := *entry
	applySelectedLink(&selected, result)
	return &selected, nil
}

func (s *concurrentState) applySelectedControl(
	control WalkControl, entry *WalkEntry, job dirJob, emit func(WalkEvent) WalkControl,
) (*WalkEntry, WalkControl) {
	if control != WalkTraverseLink {
		return entry, control
	}
	if !entry.symlink {
		err := walkErr(entry.path, entry.depth, OpReadMetadata, errNoCurrentSymlink)
		return entry, emit(WalkEvent{Err: err})
	}
	if s.opts.options.FollowLinks {
		return entry, WalkContinue
	}
	selected, err := s.prepareSelectedLink(entry, job)
	if err != nil {
		return entry, emit(WalkEvent{Err: err})
	}
	return selected, WalkContinue
}

func (s *concurrentState) selectedAncestor(path string, id platform.Identity, depth int) (bool, *WalkError) {
	for {
		info, err := platform.DirectoryInfo(path)
		if err != nil {
			return false, walkErr(path, depth, OpReadMetadata, err)
		}
		if info.Identity == id {
			return true, nil
		}
		if filepath.Clean(path) == filepath.Clean(s.abs) {
			return false, nil
		}
		parent := filepath.Dir(path)
		if parent == path || !pathx.UnderRoot(s.abs, parent) {
			return false, nil
		}
		path = parent
	}
}

func makeEntry(root, path string, depth int, info os.FileInfo, options WalkOptions, target os.FileInfo) *WalkEntry {
	source := info
	mode := info.Mode()
	symlink := mode&os.ModeSymlink != 0
	isFile := mode.IsRegular() && !symlink
	isDir := info.IsDir() && !symlink
	if symlink && options.FollowLinks {
		if target == nil {
			target, _ = os.Stat(path)
		}
		if target != nil {
			isFile = target.Mode().IsRegular()
			isDir = target.IsDir()
			info = target
		} else {
			isFile, isDir = false, false
		}
	} else if symlink {
		isFile, isDir = false, false
	}
	stat := newFileInfoCache()
	if symlink && target != nil {
		stat.load(target, nil)
	} else if !symlink {
		stat.load(source, nil)
	}
	entry := &WalkEntry{
		root: root, path: path, name: source.Name(), depth: depth,
		isFile: isFile, isDir: isDir, symlink: symlink, info: source, stat: stat,
	}
	if options.CollectMetadata && isFile {
		size := uint64(info.Size())
		entry.bytes = &size
		ver := versionFromInfo(path, info)
		entry.version = &ver
		h := platform.HiddenFromInfo(path, info)
		entry.hidden = &h
	}
	if isDir && (options.FollowLinks || options.SameFileSystem) {
		if dir, err := platform.DirectoryInfo(path); err == nil {
			id := dir.Identity
			entry.dirID = &id
		}
	}
	return entry
}

func containsID(ids []platformID, id platform.Identity) bool {
	for _, item := range ids {
		if item.fs == id.FileSystem && item.file == id.File {
			return true
		}
	}
	return false
}

type openEntries struct {
	file *os.File
	buf  []os.DirEntry
	err  error
	done bool
}

func openDirectory(path string) (*openEntries, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &openEntries{file: file}, nil
}

func (o *openEntries) next() (os.DirEntry, error, bool) {
	if o.err != nil {
		err := o.err
		o.err = nil
		return nil, err, true
	}
	if len(o.buf) == 0 {
		if o.done {
			return nil, nil, false
		}
		batch, err := o.file.ReadDir(readDirBatch)
		if err != nil && err != io.EOF {
			return nil, err, true
		}
		if len(batch) == 0 {
			o.done = true
			o.close()
			return nil, nil, false
		}
		if err == io.EOF {
			o.done = true
			_ = o.file.Close()
			o.file = nil
		}
		o.buf = batch
	}
	entry := o.buf[0]
	o.buf = o.buf[1:]
	return entry, nil, true
}

func (o *openEntries) isOpen() bool { return o.file != nil }

func (o *openEntries) close() {
	if o.file != nil {
		_ = o.file.Close()
		o.file = nil
	}
}

func (o *openEntries) drain() {
	for !o.done && o.file != nil {
		batch, err := o.file.ReadDir(readDirBatch)
		if len(batch) > 0 {
			o.buf = append(o.buf, batch...)
		}
		if err == io.EOF || len(batch) == 0 {
			o.done = true
			break
		}
		if err != nil {
			o.err = err
			o.done = true
			break
		}
	}
	o.close()
}

type bufferedEntries struct {
	items []os.DirEntry
	errs  []error
}

func (b *bufferedEntries) next() (os.DirEntry, error, bool) {
	if len(b.errs) > 0 {
		err := b.errs[0]
		b.errs = b.errs[1:]
		return nil, err, true
	}
	if len(b.items) == 0 {
		return nil, nil, false
	}
	item := b.items[0]
	b.items = b.items[1:]
	return item, nil, true
}

func (b *bufferedEntries) isOpen() bool { return false }
func (b *bufferedEntries) close()       {}
func (b *bufferedEntries) drain()       {}

func collectSorted(path string, cmp func(a, b os.DirEntry) int) (*bufferedEntries, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	if cmp != nil {
		sortDirEntries(entries, cmp)
	}
	return &bufferedEntries{items: entries}, nil
}

func sortDirEntries(entries []os.DirEntry, cmp func(a, b os.DirEntry) int) {
	n := len(entries)
	for i := 1; i < n; i++ {
		j := i
		for j > 0 && cmp(entries[j-1], entries[j]) > 0 {
			entries[j-1], entries[j] = entries[j], entries[j-1]
			j--
		}
	}
}

func childPath(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + string(os.PathSeparator) + name
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
		if filepath.Clean(dir) == filepath.Clean(root) {
			return false, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			return false, nil
		}
		dir = next
	}
	return false, nil
}
