package walk

import (
	"io"
	"os"
	"strings"

	"github.com/sergii-ziborov/treestamp/internal/platform"
)

// Builder configures serial single- or multi-root walking.
type Builder struct {
	roots         []string
	options       WalkOptions
	sorter        func(a, b os.DirEntry) int
	filter        func(*WalkEntry) bool
	skipStdout    bool
	contentsFirst bool
}

// NewBuilder starts a builder with one root.
func NewBuilder(root string) *Builder {
	return &Builder{
		roots:   []string{root},
		options: DefaultOptions(),
	}
}

func (b *Builder) AddRoot(root string) *Builder {
	b.roots = append(b.roots, root)
	return b
}

func (b *Builder) Options(options WalkOptions) *Builder {
	b.options = options
	return b
}

func (b *Builder) SortByFileName() *Builder {
	b.sorter = func(a, b os.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	}
	return b
}

func (b *Builder) ContentsFirst(enabled bool) *Builder {
	b.contentsFirst = enabled
	return b
}

func (b *Builder) SkipStdout(enabled bool) *Builder {
	b.skipStdout = enabled
	return b
}

func (b *Builder) FilterEntry(fn func(*WalkEntry) bool) *Builder {
	b.filter = fn
	return b
}

func (b *Builder) FilterDirectories(fn func(*WalkEntry) bool) *Builder {
	b.filter = func(entry *WalkEntry) bool {
		if !entry.IsDir() {
			return true
		}
		return fn(entry)
	}
	return b
}

func (b *Builder) Build() *MultiWalker {
	return &MultiWalker{
		roots:         append([]string(nil), b.roots...),
		options:       b.options,
		sorter:        b.sorter,
		filter:        b.filter,
		skipStdout:    b.skipStdout,
		contentsFirst: b.contentsFirst,
	}
}

// MultiWalker walks configured roots in order.
type MultiWalker struct {
	roots         []string
	index         int
	options       WalkOptions
	sorter        func(a, b os.DirEntry) int
	filter        func(*WalkEntry) bool
	skipStdout    bool
	contentsFirst bool
	current       *Walker
}

func (m *MultiWalker) Next() (*WalkEntry, error) {
	for {
		if m.current != nil {
			entry, err := m.current.Next()
			if err == io.EOF {
				_ = m.current.Close()
				m.current = nil
				continue
			}
			return entry, err
		}
		if m.index >= len(m.roots) {
			return nil, io.EOF
		}
		root := m.roots[m.index]
		m.index++
		var stdout *platform.Identity
		if m.skipStdout {
			if id, ok := platform.StdoutIdentity(); ok {
				stdout = &id
			}
		}
		walker, err := newWalker(root, m.options, m.sorter, m.filter, stdout, m.contentsFirst)
		if err != nil {
			return nil, err
		}
		m.current = walker
	}
}

func (m *MultiWalker) Close() error {
	if m.current != nil {
		return m.current.Close()
	}
	return nil
}

func (m *MultiWalker) SkipCurrentDir() {
	if m.current != nil {
		m.current.SkipCurrentDir()
	}
}

// Collect walks to completion. Used by tests, not a scan report.
func Collect(w interface{ Next() (*WalkEntry, error) }) ([]*WalkEntry, error) {
	var out []*WalkEntry
	for {
		entry, err := w.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, entry)
	}
}
