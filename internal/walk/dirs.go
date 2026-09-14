package walk

import (
	"io"
	"os"
	"path/filepath"

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
	// Simple insertion-free sort via slice sort.
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
	return filepath.Join(dir, name)
}
