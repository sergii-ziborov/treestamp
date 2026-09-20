// Package listwalk is the unsorted callback walk used by Walk / WalkDirs.
package listwalk

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// Entry is a persistable directory entry with cached Info/Stat.
type Entry struct {
	name, path string
	typ        fs.FileMode
	depth      int
	source     fs.DirEntry
	ready      fs.FileInfo
	info       atomic.Pointer[statCache]
	stat       atomic.Pointer[statCache]
}

type statCache struct {
	once sync.Once
	info fs.FileInfo
	err  error
}

var pool = sync.Pool{New: func() any { return &Entry{} }}

// Own returns a persistable entry. The caller may keep it; do not Release it.
func Own(name, path string, typ fs.FileMode, depth int, ready fs.FileInfo) *Entry {
	return &Entry{name: name, path: path, typ: typ, depth: depth, ready: ready}
}

// Put fills dst with a persistable entry. The caller owns dst.
func Put(dst *Entry, name, path string, typ fs.FileMode, depth int, ready fs.FileInfo) {
	if dst != nil {
		*dst = Entry{name: name, path: path, typ: typ, depth: depth, ready: ready}
	}
}

// Deliver invokes fn with the owned entry. The callback may keep it.
func Deliver(fn fs.WalkDirFunc, entry *Entry, toSlash bool) error {
	return fn(Show(entry.path, toSlash), entry, nil)
}

// Acquire returns a pooled entry. Release it after the callback returns.
func Acquire(name, path string, typ fs.FileMode, depth int, ready fs.FileInfo) *Entry {
	entry := pool.Get().(*Entry)
	*entry = Entry{name: name, path: path, typ: typ, depth: depth, ready: ready}
	return entry
}

// Release returns the entry to the pool.
func Release(entry *Entry) {
	if entry != nil {
		*entry = Entry{}
		pool.Put(entry)
	}
}

func (e *Entry) Name() string      { return e.name }
func (e *Entry) Path() string      { return e.path }
func (e *Entry) IsDir() bool       { return e.typ.IsDir() }
func (e *Entry) Type() fs.FileMode { return e.typ }
func (e *Entry) Depth() int        { return e.depth }

// Bind attaches the OS directory entry so Info can reuse its cached metadata.
func (e *Entry) Bind(dent fs.DirEntry) { e.source = dent }

func (e *Entry) Info() (fs.FileInfo, error) {
	if e.ready != nil {
		return e.ready, nil
	}
	return e.loadInfo().do(func() (fs.FileInfo, error) {
		if e.source != nil {
			return e.source.Info()
		}
		return os.Lstat(e.path)
	})
}

func (e *Entry) Stat() (fs.FileInfo, error) {
	if e.typ&os.ModeSymlink == 0 {
		return e.Info()
	}
	return e.loadStat().do(func() (fs.FileInfo, error) {
		return os.Stat(e.path)
	})
}

func (e *Entry) loadInfo() *statCache { return loadCache(&e.info) }
func (e *Entry) loadStat() *statCache { return loadCache(&e.stat) }

func loadCache(slot *atomic.Pointer[statCache]) *statCache {
	if c := slot.Load(); c != nil {
		return c
	}
	c := &statCache{}
	if !slot.CompareAndSwap(nil, c) {
		return slot.Load()
	}
	return c
}

func (c *statCache) do(load func() (fs.FileInfo, error)) (fs.FileInfo, error) {
	c.once.Do(func() { c.info, c.err = load() })
	return c.info, c.err
}

// Show returns a native or slash path for a callback.
func Show(path string, toSlash bool) string {
	if toSlash {
		return filepath.ToSlash(path)
	}
	return path
}

// Clone is a persistable copy. The caller may keep it after Release.
func (e *Entry) Clone() *Entry {
	if e == nil {
		return nil
	}
	out := &Entry{name: e.name, path: e.path, typ: e.typ, depth: e.depth, source: e.source, ready: e.ready}
	if c := e.info.Load(); c != nil {
		out.info.Store(c)
	}
	if c := e.stat.Load(); c != nil {
		out.stat.Store(c)
	}
	return out
}

// Call invokes fn with a persistable entry and no error.
func Call(fn fs.WalkDirFunc, entry *Entry, toSlash bool) error {
	return fn(Show(entry.path, toSlash), entry.Clone(), nil)
}
