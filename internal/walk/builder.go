package walk

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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
	b.filter = func(entry *WalkEntry) bool { return !entry.IsDir() || fn(entry) }
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
	if m.current != nil { return m.current.Close() }
	return nil
}

func (m *MultiWalker) SkipCurrentDir() {
	if m.current != nil { m.current.SkipCurrentDir() }
}

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
		entry := makeEntry(w.root, w.root, 0, info, w.options, nil)
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
	if p.close != nil { return p.close() }
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
		entry := makeEntry(spec.root, path, spec.depth+1, info, spec.options, nil)
		item := &StatefulWalkEntry[E]{Entry: entry, ReadChildren: entry.isDir && !entry.hasSkip()}
		batch = append(batch, StatefulResult[E]{Entry: item})
	}
	if process != nil {
		process(spec.depth, spec.dir, &state, &batch)
	}
	return batch, state, nil
}

func (p *ParallelWalker) Walk() (*ParallelWalkReport, error) {
	var mu sync.Mutex
	report := &ParallelWalkReport{}
	_, err := p.Visit(func(ev WalkEvent) WalkControl {
		mu.Lock()
		defer mu.Unlock()
		if ev.Err != nil {
			report.Errors = append(report.Errors, ev.Err)
			if p.options.ErrorPolicy == ErrorAbort {
				return WalkQuit
			}
			return WalkContinue
		}
		clone := *ev.Entry
		report.Entries = append(report.Entries, &clone)
		return WalkContinue
	})
	if err != nil {
		return report, err
	}
	if p.options.ErrorPolicy == ErrorAbort && len(report.Errors) > 0 {
		return report, report.Errors[0]
	}
	sort.Slice(report.Entries, func(i, j int) bool { return report.Entries[i].path < report.Entries[j].path })
	return report, nil
}

func WalkParallel(root string, workers int, fn WalkFunc) error {
	var mu sync.Mutex
	var fnErr error
	_, err := NewParallelWalker(root).WithParallelism(workers).Visit(func(ev WalkEvent) WalkControl {
		if ev.Err != nil {
			return WalkContinue
		}
		if err := fn(ev.Entry); err != nil {
			if errors.Is(err, ErrTraverseLink) && ev.Entry.symlink {
				return WalkTraverseLink
			}
			mu.Lock()
			if fnErr == nil && err != io.EOF {
				fnErr = err
			}
			mu.Unlock()
			return WalkQuit
		}
		return WalkContinue
	})
	if fnErr != nil {
		return fnErr
	}
	return err
}

func CollectParallel(root string, workers int, sortPaths bool) ([]*WalkEntry, error) {
	walker := NewParallelWalker(root).WithParallelism(workers)
	if sortPaths {
		report, err := walker.Walk()
		if err != nil {
			return nil, err
		}
		return report.Entries, nil
	}
	var mu sync.Mutex
	var entries []*WalkEntry
	_, err := walker.Visit(func(ev WalkEvent) WalkControl {
		if ev.Err != nil {
			return WalkContinue
		}
		clone := *ev.Entry
		mu.Lock()
		entries = append(entries, &clone)
		mu.Unlock()
		return WalkContinue
	})
	return entries, err
}

type ParallelWalkIter struct {
	ch     chan item
	token  *cancelFlag
	once   sync.Once
	closed bool
	err    error
}

func (it *ParallelWalkIter) Err() error {
	if it == nil { return nil }
	return it.err
}

type item struct {
	entry *WalkEntry
	err   error
}

type cancelFlag struct {
	mu   sync.Mutex
	done bool
	fn   func()
}

func (c *cancelFlag) cancel() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.done {
		c.done = true
		if c.fn != nil {
			c.fn()
		}
	}
}

func (p *ParallelWalker) IntoIterOrderedBounded(capacity int) *ParallelWalkIter {
	iter, err := p.TryIntoIterOrderedBounded(capacity)
	if err != nil {
		return &ParallelWalkIter{closed: true, err: err}
	}
	return iter
}

func (p *ParallelWalker) IntoIterBounded(capacity int) *ParallelWalkIter {
	if capacity < 1 {
		capacity = 1
	}
	ch := make(chan item, capacity)
	quit := make(chan struct{})
	flag := &cancelFlag{fn: func() { close(quit) }}
	iter := &ParallelWalkIter{ch: ch, token: flag}
	if err := p.runtime.AdmitWait(func() {
		defer close(ch)
		_, _ = p.Visit(func(ev WalkEvent) WalkControl {
			select {
			case <-quit:
				return WalkQuit
			default:
			}
			var it item
			if ev.Err != nil {
				it.err = ev.Err
			} else {
				clone := *ev.Entry
				it.entry = &clone
			}
			select {
			case <-quit:
				return WalkQuit
			case ch <- it:
				return WalkContinue
			}
		})
	}); err != nil {
		close(ch)
		iter.closed = true
		iter.err = err
	}
	return iter
}

func (it *ParallelWalkIter) Next() (*WalkEntry, error) {
	if it == nil { return nil, io.EOF }
	if it.closed {
		if it.err != nil { return nil, it.err }
		return nil, io.EOF
	}
	got, ok := <-it.ch
	if !ok {
		it.closed = true
		return nil, io.EOF
	}
	if got.err != nil {
		return nil, got.err
	}
	return got.entry, nil
}

func (it *ParallelWalkIter) Close() error {
	if it == nil {
		return nil
	}
	it.once.Do(func() {
		if it.token != nil {
			it.token.cancel()
		}
		if it.ch != nil {
			for range it.ch {
			}
		}
		it.closed = true
	})
	return nil
}

func newDirQueue() *dirQueue {
	q := &dirQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *dirQueue) push(job dirJob) {
	q.mu.Lock()
	if !q.closed {
		q.items = append(q.items, job)
		q.cond.Signal()
	}
	q.mu.Unlock()
}

func (q *dirQueue) pop() (dirJob, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		if q.closed {
			return dirJob{}, false
		}
		if len(q.items) > 0 {
			job := q.items[0]
			q.items = q.items[1:]
			q.inflight++
			return job, true
		}
		if q.inflight == 0 {
			q.closed = true
			q.cond.Broadcast()
			return dirJob{}, false
		}
		q.cond.Wait()
	}
}

func (q *dirQueue) done() {
	q.mu.Lock()
	q.inflight--
	if q.inflight == 0 && len(q.items) == 0 {
		q.closed = true
		q.cond.Broadcast()
	} else {
		q.cond.Signal()
	}
	q.mu.Unlock()
}

func (q *dirQueue) close() {
	q.mu.Lock(); q.closed = true; q.cond.Broadcast(); q.mu.Unlock()
}

func (q *dirQueue) takeAll() []dirJob {
	q.mu.Lock(); items := q.items; q.items = nil; q.mu.Unlock(); return items
}

func orderDirents(entries []os.DirEntry, opts WalkOptions) []os.DirEntry {
	if opts.LocalSort != LocalSortNone {
		sortDirents(entries, opts.LocalSort)
	}
	if !opts.ContentsFirst && !opts.DirsFirst {
		return entries
	}
	files, dirs, other := splitDirents(entries)
	if opts.ContentsFirst {
		return append(append(files, other...), dirs...)
	}
	return append(append(dirs, other...), files...)
}

func emitOrdered(state *concurrentState, p *orderedPull, ch chan item) {
	if state.rootEntry == nil {
		return
	}
	clone := *state.rootEntry
	if !sendOrdered(p.quit, ch, item{entry: &clone}) || !state.rootEntry.isDir || state.rootEntry.hasSkip() {
		return
	}
	job := dirJob{path: state.abs}
	if id := state.rootEntry.dirID; id != nil {
		job.ancestors = []platformID{{fs: id.FileSystem, file: id.File}}
	}
	p.addJob(job, true)
	p.emitDir(state.abs, ch)
}

func (p *orderedPull) forget(path string) {
	p.mu.Lock()
	got := p.listed[path]
	delete(p.listed, path)
	delete(p.jobs, path)
	p.mu.Unlock()
	if got != nil && got.held {
		got.held = false
		p.budget.DropReady(got.bytes)
	}
}

func splitDirents(entries []os.DirEntry) (files, dirs, other []os.DirEntry) {
	for _, dent := range entries {
		switch {
		case dent.Type().IsRegular():
			files = append(files, dent)
		case dent.IsDir() || dent.Type()&os.ModeDir != 0:
			dirs = append(dirs, dent)
		default:
			other = append(other, dent)
		}
	}
	return files, dirs, other
}
