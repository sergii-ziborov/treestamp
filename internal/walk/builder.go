package walk

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sergii-ziborov/treestamp/internal/platform"
)

type Builder struct {
	roots         []string
	options       WalkOptions
	sorter        func(a, b os.DirEntry) int
	filter        func(*WalkEntry) bool
	skipStdout    bool
	contentsFirst bool
}

func NewBuilder(root string) *Builder {
	return &Builder{roots: []string{root}, options: DefaultOptions()}
}

func (b *Builder) AddRoot(root string) *Builder         { b.roots = append(b.roots, root); return b }
func (b *Builder) Options(options WalkOptions) *Builder { b.options = options; return b }
func (b *Builder) SortByFileName() *Builder             { return b.SortByName() }
func (b *Builder) SortByName() *Builder {
	return b.SortBy(func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
}
func (b *Builder) SortBy(fn func(a, b os.DirEntry) int) *Builder { b.sorter = fn; return b }
func (b *Builder) ContentsFirst(enabled bool) *Builder           { b.contentsFirst = enabled; return b }
func (b *Builder) SkipStdout(enabled bool) *Builder              { b.skipStdout = enabled; return b }
func (b *Builder) FilterEntry(fn func(*WalkEntry) bool) *Builder { b.filter = fn; return b }
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
		roots: append([]string(nil), b.roots...), options: b.options, sorter: b.sorter,
		filter: b.filter, skipStdout: b.skipStdout, contentsFirst: b.contentsFirst,
	}
}

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
		walker, err := newWalker(root, m.options, walkerConfig{
			sorter: m.sorter, filter: m.filter, skipStdout: stdout, contentsFirst: m.contentsFirst,
		})
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

// TraverseCurrentSymlink follows the symlink returned by the last Next call.
func (m *MultiWalker) TraverseCurrentSymlink() error {
	if m.current == nil {
		return walkErr("", 0, OpReadMetadata, errNoCurrentSymlink)
	}
	return m.current.TraverseCurrentSymlink()
}

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

type StatefulWalkEntry[E any] struct {
	Entry        *WalkEntry
	State        E
	ReadChildren bool
}

func (e *StatefulWalkEntry[E]) Path() string { return e.Entry.Path() }
func (e *StatefulWalkEntry[E]) Depth() int   { return e.Entry.Depth() }
func (e *StatefulWalkEntry[E]) IsFile() bool { return e.Entry.IsFile() }
func (e *StatefulWalkEntry[E]) IsDir() bool  { return e.Entry.IsDir() }
func (e *StatefulWalkEntry[E]) SetReadChildren(enabled bool) {
	e.ReadChildren = enabled && e.Entry.IsDir() && !e.Entry.hasSkip()
}

type ProcessReadDir[R, E any] func(depth int, dir string, state *R, batch *[]StatefulResult[E])
type StatefulResult[E any] struct {
	Entry *StatefulWalkEntry[E]
	Err   error
}

type StatefulWalkBuilder[R, E any] struct {
	root      string
	options   WalkOptions
	rootState R
	process   ProcessReadDir[R, E]
	workers   int
}

func NewStatefulWalkBuilder[R, E any](root string, rootState R) *StatefulWalkBuilder[R, E] {
	return &StatefulWalkBuilder[R, E]{root: root, options: DefaultOptions(), rootState: rootState}
}

func (b *StatefulWalkBuilder[R, E]) Options(options WalkOptions) *StatefulWalkBuilder[R, E] {
	b.options = options
	return b
}
func (b *StatefulWalkBuilder[R, E]) WithParallelism(n int) *StatefulWalkBuilder[R, E] {
	b.workers = n
	return b
}
func (b *StatefulWalkBuilder[R, E]) ProcessReadDir(fn ProcessReadDir[R, E]) *StatefulWalkBuilder[R, E] {
	b.process = fn
	return b
}
func (b *StatefulWalkBuilder[R, E]) Build() (*StatefulWalker[R, E], error) {
	return newStatefulWalker(b)
}
func (b *StatefulWalkBuilder[R, E]) BuildParallelOrdered(capacity int) (*ParallelStatefulWalker[E], error) {
	serial, err := newStatefulWalker(b)
	if err != nil {
		return nil, err
	}
	_ = capacity
	return &ParallelStatefulWalker[E]{next: serial.Next, close: serial.Close}, nil
}

type frame[R, E any] struct {
	items []StatefulResult[E]
	index int
	state R
}

type StatefulWalker[R, E any] struct {
	root      string
	options   WalkOptions
	process   ProcessReadDir[R, E]
	rootState R
	rootOnce  bool
	frames    []frame[R, E]
	finished  bool
}

func newStatefulWalker[R, E any](b *StatefulWalkBuilder[R, E]) (*StatefulWalker[R, E], error) {
	abs, err := filepath.Abs(b.root)
	if err != nil {
		return nil, walkErr(b.root, 0, OpCanonicalize, err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, walkErr(abs, 0, OpReadMetadata, err)
	}
	w := &StatefulWalker[R, E]{root: abs, options: b.options.Normalize(), process: b.process, rootState: b.rootState, rootOnce: true}
	if info.IsDir() {
		batch, state, batchErr := readBatch[R, E](batchSpec{root: abs, dir: abs, depth: 0, options: w.options}, w.rootState, w.process)
		if batchErr != nil {
			return nil, batchErr
		}
		w.frames = []frame[R, E]{{items: batch, state: state}}
	}
	return w, nil
}

func (w *StatefulWalker[R, E]) Next() (*StatefulWalkEntry[E], error) {
	if w.finished {
		return nil, io.EOF
	}
	if w.rootOnce {
		w.rootOnce = false
		info, err := os.Lstat(w.root)
		if err != nil {
			return nil, walkErr(w.root, 0, OpReadMetadata, err)
		}
		entry := makeEntry(w.root, w.root, 0, info, w.options)
		var state E
		return &StatefulWalkEntry[E]{Entry: entry, State: state, ReadChildren: entry.isDir}, nil
	}
	for len(w.frames) > 0 {
		top := &w.frames[len(w.frames)-1]
		if top.index >= len(top.items) {
			w.frames = w.frames[:len(w.frames)-1]
			continue
		}
		item := top.items[top.index]
		top.index++
		if item.Err != nil {
			return nil, item.Err
		}
		if item.Entry.ReadChildren && item.Entry.Entry.IsDir() {
			batch, state, err := readBatch[R, E](batchSpec{root: w.root, dir: item.Entry.Entry.Path(), depth: item.Entry.Entry.Depth(), options: w.options}, top.state, w.process)
			if err != nil {
				return item.Entry, err
			}
			w.frames = append(w.frames, frame[R, E]{items: batch, state: state})
		}
		return item.Entry, nil
	}
	w.finished = true
	return nil, io.EOF
}

func (w *StatefulWalker[R, E]) Close() error {
	w.finished = true
	w.frames = nil
	return nil
}

type ParallelStatefulWalker[E any] struct {
	next  func() (*StatefulWalkEntry[E], error)
	close func() error
}

func (p *ParallelStatefulWalker[E]) Next() (*StatefulWalkEntry[E], error) {
	if p == nil || p.next == nil {
		return nil, io.EOF
	}
	return p.next()
}
func (p *ParallelStatefulWalker[E]) Close() error {
	if p.close != nil {
		return p.close()
	}
	return nil
}

type batchSpec struct {
	root, dir string
	depth     int
	options   WalkOptions
}

func readBatch[R, E any](spec batchSpec, state R, process ProcessReadDir[R, E]) ([]StatefulResult[E], R, error) {
	dents, err := os.ReadDir(spec.dir)
	if err != nil {
		return nil, state, walkErr(spec.dir, spec.depth+1, OpReadDirectory, err)
	}
	batch := make([]StatefulResult[E], 0, len(dents))
	for _, dent := range dents {
		path := filepath.Join(spec.dir, dent.Name())
		info, infoErr := dent.Info()
		if infoErr != nil {
			batch = append(batch, StatefulResult[E]{Err: walkErr(path, spec.depth+1, OpReadMetadata, infoErr)})
			continue
		}
		entry := makeEntry(spec.root, path, spec.depth+1, info, spec.options)
		item := &StatefulWalkEntry[E]{Entry: entry, ReadChildren: entry.isDir && !entry.hasSkip()}
		batch = append(batch, StatefulResult[E]{Entry: item})
	}
	if process != nil {
		process(spec.depth, spec.dir, &state, &batch)
	}
	return batch, state, nil
}
