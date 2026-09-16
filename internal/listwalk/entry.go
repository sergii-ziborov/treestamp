// Package listwalk is the unsorted callback walk used by Walk / WalkDirs.
package listwalk

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// Entry is a pooled directory entry with cached Info/Stat.
type Entry struct {
	name, path string
	typ        fs.FileMode
	depth      int
	source     fs.DirEntry
	ready      fs.FileInfo
	info, stat *statCache
}

type statCache struct {
	once sync.Once
	info fs.FileInfo
	err  error
}

var pool = sync.Pool{New: func() any { return &Entry{} }}

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
	if e.source != nil {
		info, err := e.source.Info()
		if err == nil {
			e.ready = info
		}
		return info, err
	}
	if e.info == nil {
		e.info = &statCache{}
	}
	return e.info.load(e.path, true)
}

func (e *Entry) Stat() (fs.FileInfo, error) {
	if e.typ&os.ModeSymlink == 0 {
		return e.Info()
	}
	if e.stat == nil {
		e.stat = &statCache{}
	}
	return e.stat.load(e.path, false)
}

func (c *statCache) load(path string, lstat bool) (fs.FileInfo, error) {
	c.once.Do(func() {
		if lstat {
			c.info, c.err = os.Lstat(path)
			return
		}
		c.info, c.err = os.Stat(path)
	})
	return c.info, c.err
}

// Show returns a native or slash path for a callback.
func Show(path string, toSlash bool) string {
	if toSlash {
		return filepath.ToSlash(path)
	}
	return path
}

// Call invokes fn with the entry and no error.
func Call(fn fs.WalkDirFunc, entry *Entry, toSlash bool) error {
	return fn(Show(entry.path, toSlash), entry, nil)
}
