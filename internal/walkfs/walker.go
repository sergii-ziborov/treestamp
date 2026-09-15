// Package walkfs traverses arbitrary io/fs filesystems.
package walkfs

import (
	"io"
	"io/fs"
	"path"
	"strings"
)

// Entry adds traversal context to an fs.DirEntry.
type Entry struct {
	root  string
	path  string
	depth int
	entry fs.DirEntry
}

func (e *Entry) Path() string               { return e.path }
func (e *Entry) FileName() string           { return e.entry.Name() }
func (e *Entry) Depth() int                 { return e.depth }
func (e *Entry) IsDir() bool                { return e.entry.IsDir() }
func (e *Entry) IsFile() bool               { return e.entry.Type().IsRegular() }
func (e *Entry) IsSymlink() bool            { return e.entry.Type()&fs.ModeSymlink != 0 }
func (e *Entry) Type() fs.FileMode          { return e.entry.Type() }
func (e *Entry) Info() (fs.FileInfo, error) { return e.entry.Info() }
func (e *Entry) DirEntry() fs.DirEntry      { return e.entry }

func (e *Entry) RelativePath() string {
	if e.path == e.root {
		return ""
	}
	if e.root == "." {
		return e.path
	}
	return strings.TrimPrefix(e.path, e.root+"/")
}

type dirFrame struct {
	path    string
	depth   int
	entries []fs.DirEntry
	index   int
}

// Walker is a deterministic depth-first pull walker over an fs.FS.
type Walker struct {
	fsys             fs.FS
	root             string
	rootEntry        *Entry
	frames           []dirFrame
	pending          *Entry
	current          *Entry
	yieldRoot        bool
	skipPending      bool
	currentReadError bool
	closed           bool
}

// New constructs a pull walker rooted at a valid io/fs path.
func New(fsys fs.FS, root string) (*Walker, error) {
	if fsys == nil || !fs.ValidPath(root) {
		return nil, invalidPath(root)
	}
	info, err := fs.Stat(fsys, root)
	if err != nil {
		return nil, err
	}
	entry := newEntry(root, root, 0, fs.FileInfoToDirEntry(info))
	return &Walker{fsys: fsys, root: root, rootEntry: entry, yieldRoot: true}, nil
}

func invalidPath(name string) error {
	return &fs.PathError{Op: "walk", Path: name, Err: fs.ErrInvalid}
}

func newEntry(root, name string, depth int, entry fs.DirEntry) *Entry {
	return &Entry{root: root, path: name, depth: depth, entry: entry}
}

func (w *Walker) Root() string { return w.root }
func (w *Walker) FS() fs.FS    { return w.fsys }

// Next returns the next entry. A directory read error returns that directory
// again with the error, matching fs.WalkDir callback behavior.
func (w *Walker) Next() (*Entry, error) {
	w.current = nil
	w.currentReadError = false
	if w.closed {
		return nil, io.EOF
	}
	if w.yieldRoot {
		w.yieldRoot = false
		w.current = w.rootEntry
		if w.rootEntry.IsDir() {
			w.pending = w.rootEntry
		}
		return w.rootEntry, nil
	}
	if entry, err := w.schedulePending(); err != nil {
		w.current, w.currentReadError = entry, true
		return entry, err
	}
	for len(w.frames) > 0 {
		top := &w.frames[len(w.frames)-1]
		if top.index >= len(top.entries) {
			w.frames = w.frames[:len(w.frames)-1]
			continue
		}
		dirEntry := top.entries[top.index]
		top.index++
		entry := newEntry(w.root, path.Join(top.path, dirEntry.Name()), top.depth+1, dirEntry)
		w.current = entry
		if entry.IsDir() {
			w.pending = entry
		}
		return entry, nil
	}
	w.closed = true
	return nil, io.EOF
}

func (w *Walker) schedulePending() (*Entry, error) {
	if w.pending == nil {
		return nil, nil
	}
	pending := w.pending
	w.pending = nil
	if w.skipPending {
		w.skipPending = false
		return nil, nil
	}
	entries, err := fs.ReadDir(w.fsys, pending.path)
	if len(entries) > 0 {
		w.frames = append(w.frames, dirFrame{
			path: pending.path, depth: pending.depth, entries: entries,
		})
	}
	return pending, err
}

// SkipCurrentDir prevents descent into the current directory.
func (w *Walker) SkipCurrentDir() {
	if w.current == nil || !w.current.IsDir() {
		return
	}
	if w.currentReadError {
		w.dropCurrentFrame()
		return
	}
	if w.pending != nil && w.pending.path == w.current.path {
		w.skipPending = true
	}
}

func (w *Walker) dropCurrentFrame() {
	if len(w.frames) == 0 {
		return
	}
	last := len(w.frames) - 1
	if w.frames[last].path == w.current.path {
		w.frames = w.frames[:last]
	}
}

func (w *Walker) skipCurrentParent() {
	if len(w.frames) > 0 {
		w.frames[len(w.frames)-1].index = len(w.frames[len(w.frames)-1].entries)
	}
}

// Close releases traversal state.
func (w *Walker) Close() error {
	w.closed = true
	w.frames = nil
	w.pending = nil
	w.current = nil
	return nil
}

// Walk traverses fsys with the callback contract of fs.WalkDir.
func Walk(fsys fs.FS, root string, fn fs.WalkDirFunc) error {
	if fn == nil {
		return invalidPath(root)
	}
	walker, err := New(fsys, root)
	if err != nil {
		return finish(fn(root, nil, err))
	}
	defer walker.Close()
	for {
		entry, nextErr := walker.Next()
		if nextErr == io.EOF {
			return nil
		}
		if nextErr != nil {
			if err := handleReadError(walker, entry, nextErr, fn); err != nil {
				return finish(err)
			}
			continue
		}
		if err := handleEntry(walker, entry, fn); err != nil {
			return finish(err)
		}
	}
}

func handleReadError(walker *Walker, entry *Entry, readErr error, fn fs.WalkDirFunc) error {
	err := fn(entry.Path(), entry.DirEntry(), readErr)
	if err == fs.SkipDir {
		walker.SkipCurrentDir()
		return nil
	}
	return err
}

func handleEntry(walker *Walker, entry *Entry, fn fs.WalkDirFunc) error {
	err := fn(entry.Path(), entry.DirEntry(), nil)
	if err != fs.SkipDir {
		return err
	}
	if entry.IsDir() {
		walker.SkipCurrentDir()
	} else {
		walker.skipCurrentParent()
	}
	return nil
}

func finish(err error) error {
	if err == fs.SkipDir || err == fs.SkipAll {
		return nil
	}
	return err
}
