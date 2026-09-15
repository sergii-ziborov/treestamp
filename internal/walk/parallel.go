package walk

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	pathx "github.com/sergii-ziborov/treestamp/internal/path"
	"github.com/sergii-ziborov/treestamp/internal/platform"
	rtruntime "github.com/sergii-ziborov/treestamp/internal/runtime"
)

type ParallelWalker struct {
	root       string
	options    WalkOptions
	workers    int
	skipStdout bool
	runtime    rtruntime.Runtime
}

func NewParallelWalker(root string) *ParallelWalker {
	return &ParallelWalker{root: root, options: DefaultOptions(), runtime: rtruntime.Global()}
}

func (p *ParallelWalker) Options(options WalkOptions) *ParallelWalker  { p.options = options; return p }
func (p *ParallelWalker) WithParallelism(n int) *ParallelWalker        { p.workers = n; return p }
func (p *ParallelWalker) SkipStdout(enabled bool) *ParallelWalker      { p.skipStdout = enabled; return p }
func (p *ParallelWalker) Runtime(rt rtruntime.Runtime) *ParallelWalker { p.runtime = rt; return p }

func (p *ParallelWalker) workerCount() int {
	n := p.workers
	if n <= 0 {
		n = p.runtime.Parallelism()
		if n > 8 {
			n = 8
		}
	}
	if n < 1 {
		n = 1
	}
	if p.options.MaxOpen > 0 && n > p.options.MaxOpen {
		n = p.options.MaxOpen
	}
	return n
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

func (p *ParallelWalker) Visit(fn ControlFunc) (*ParallelVisitReport, error) {
	return p.VisitWithToken(nil, fn)
}

func (p *ParallelWalker) VisitWithToken(token *rtruntime.Token, fn ControlFunc) (*ParallelVisitReport, error) {
	return walkConcurrent(p.root, concurrentOpts{options: p.options.Normalize(), workers: p.workerCount(), skipStdout: p.skipStdout, token: token}, fn)
}

func WalkParallel(root string, workers int, fn WalkFunc) error {
	var fnErr error
	_, err := NewParallelWalker(root).WithParallelism(workers).Visit(func(ev WalkEvent) WalkControl {
		if ev.Err != nil {
			return WalkContinue
		}
		if err := fn(ev.Entry); err != nil {
			if errors.Is(err, ErrTraverseLink) && ev.Entry.symlink {
				return WalkTraverseLink
			}
			fnErr = err
			if err == io.EOF {
				fnErr = nil
			}
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
	if !sortPaths {
		return collectParallelUnsorted(walker)
	}
	report, err := walker.Walk()
	if err != nil {
		return nil, err
	}
	return report.Entries, nil
}

func collectParallelUnsorted(walker *ParallelWalker) ([]*WalkEntry, error) {
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

type concurrentOpts struct {
	options    WalkOptions
	workers    int
	skipStdout bool
	token      *rtruntime.Token
}

type concurrentState struct {
	abs       string
	opts      concurrentOpts
	stdout    *platform.Identity
	rootFS    uint64
	haveFS    bool
	report    *ParallelVisitReport
	first     error
	quit      atomic.Bool
	mu        sync.Mutex
	rootEntry *WalkEntry
}

func walkConcurrent(root string, opts concurrentOpts, fn ControlFunc) (*ParallelVisitReport, error) {
	state, err := setupConcurrent(root, opts)
	if err != nil || state.rootEntry == nil {
		if state != nil {
			return state.report, err
		}
		return nil, err
	}
	emit := state.makeEmit(fn)
	if emit(WalkEvent{Entry: state.rootEntry}) == WalkQuit || !state.rootEntry.isDir || state.rootEntry.hasSkip() {
		return state.report, state.first
	}
	queue := newDirQueue()
	var ancestors []platformID
	if state.rootEntry.dirID != nil {
		ancestors = []platformID{{fs: state.rootEntry.dirID.FileSystem, file: state.rootEntry.dirID.File}}
	}
	queue.push(dirJob{path: state.abs, depth: 0, ancestors: ancestors})
	var wg sync.WaitGroup
	wg.Add(opts.workers)
	for i := 0; i < opts.workers; i++ {
		go func() {
			defer wg.Done()
			for !state.quit.Load() {
				job, ok := queue.pop()
				if !ok {
					return
				}
				state.processJob(job, queue, emit)
			}
			queue.close()
		}()
	}
	wg.Wait()
	return state.report, state.first
}

func setupConcurrent(root string, opts concurrentOpts) (*concurrentState, error) {
	if opts.workers <= 0 {
		opts.workers = runtime.GOMAXPROCS(0)
		if opts.workers < 1 {
			opts.workers = 1
		}
	}
	opts.options = opts.options.Normalize()
	abs, info, err := concurrentRoot(root, opts.options)
	if err != nil {
		return nil, err
	}
	state := &concurrentState{abs: abs, opts: opts, report: &ParallelVisitReport{}}
	if opts.skipStdout {
		if id, ok := platform.StdoutIdentity(); ok {
			state.stdout = &id
		}
	}
	if opts.options.SameFileSystem && info.IsDir() {
		dir, dirErr := platform.DirectoryInfo(abs)
		if dirErr != nil {
			return nil, walkErr(abs, 0, OpReadMetadata, dirErr)
		}
		state.rootFS, state.haveFS = dir.FileSystem, true
	}
	state.rootEntry = makeEntry(abs, abs, 0, info, opts.options)
	if state.stdout != nil && state.rootEntry.isFile {
		if match, _ := platform.PathMatchesIdentity(abs, *state.stdout); match {
			state.rootEntry = nil
		}
	}
	return state, nil
}

func concurrentRoot(root string, options WalkOptions) (string, os.FileInfo, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", nil, walkErr(root, 0, OpCanonicalize, err)
	}
	linkInfo, err := os.Lstat(abs)
	if err != nil {
		return "", nil, walkErr(abs, 0, OpReadMetadata, err)
	}
	if options.RootSymlinkPolicy == RootReject && linkInfo.Mode()&os.ModeSymlink != 0 {
		return "", nil, walkErr(abs, 0, OpReadMetadata, errRootSymlink)
	}
	if options.FollowLinks || options.SameFileSystem {
		resolved, resolveErr := filepath.EvalSymlinks(abs)
		if resolveErr != nil {
			return "", nil, walkErr(abs, 0, OpCanonicalize, resolveErr)
		}
		abs = resolved
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", nil, walkErr(abs, 0, OpReadMetadata, err)
	}
	return abs, info, nil
}

func (s *concurrentState) makeEmit(fn ControlFunc) func(WalkEvent) WalkControl {
	return func(ev WalkEvent) WalkControl {
		if s.opts.token.Cancelled() {
			s.report.Cancelled = true
			s.quit.Store(true)
			return WalkQuit
		}
		control := fn(ev)
		s.mu.Lock()
		if ev.Err != nil {
			s.report.Errors = append(s.report.Errors, ev.Err)
			if s.opts.options.ErrorPolicy == ErrorAbort && s.first == nil {
				s.first = ev.Err
				s.quit.Store(true)
				control = WalkQuit
			}
		} else {
			s.report.Visited++
		}
		if control == WalkQuit {
			s.report.Quit = true
			s.quit.Store(true)
		}
		s.mu.Unlock()
		return control
	}
}

func (s *concurrentState) processJob(job dirJob, queue *dirQueue, emit func(WalkEvent) WalkControl) {
	entries, readErr := os.ReadDir(job.path)
	if readErr != nil {
		emit(WalkEvent{Err: walkErr(job.path, job.depth+1, OpReadDirectory, readErr)})
		queue.done()
		return
	}
	for _, dent := range entries {
		if s.quit.Load() {
			break
		}
		if s.handleDirent(job, dent, queue, emit) == WalkQuit {
			break
		}
	}
	queue.done()
}

func (s *concurrentState) handleDirent(job dirJob, dent os.DirEntry, queue *dirQueue, emit func(WalkEvent) WalkControl) WalkControl {
	path := filepath.Join(job.path, dent.Name())
	info, infoErr := dent.Info()
	if infoErr != nil {
		return emit(WalkEvent{Err: walkErr(path, job.depth+1, OpReadMetadata, infoErr)})
	}
	if info.Mode()&os.ModeSymlink != 0 && s.opts.options.FollowLinks {
		if _, targetErr := os.Stat(path); targetErr != nil {
			return emit(WalkEvent{Err: walkErr(path, job.depth+1, OpReadMetadata, targetErr)})
		}
	}
	entry := makeEntry(s.abs, path, job.depth+1, info, s.opts.options)
	if s.stdout != nil && entry.isFile {
		if match, _ := platform.PathMatchesIdentity(path, *s.stdout); match {
			return WalkContinue
		}
	}
	if policyErr := s.applyDirPolicy(entry, path, job); policyErr != nil {
		return emit(WalkEvent{Err: policyErr})
	}
	control := emit(WalkEvent{Entry: entry})
	entry, control = s.applySelectedControl(control, entry, job, emit)
	if control == WalkQuit || control == WalkSkip || !entry.isDir || entry.hasSkip() {
		return control
	}
	nextAnc := job.ancestors
	if entry.dirID != nil {
		nextAnc = append(append([]platformID(nil), job.ancestors...), platformID{fs: entry.dirID.FileSystem, file: entry.dirID.File})
	}
	queue.push(dirJob{path: path, depth: entry.depth, ancestors: nextAnc})
	return control
}

func (s *concurrentState) applyDirPolicy(entry *WalkEntry, path string, job dirJob) *WalkError {
	options := s.opts.options
	if entry.symlink && options.FollowLinks {
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			return walkErr(path, entry.depth, OpCanonicalize, err)
		}
		if !pathx.UnderRoot(s.abs, canonical) {
			entry.skip = SkipPathEscape
			return nil
		}
	}
	if entry.isDir && (options.FollowLinks || options.SameFileSystem) {
		dir, err := platform.DirectoryInfo(path)
		if err != nil {
			return walkErr(path, entry.depth, OpReadMetadata, err)
		}
		if options.SameFileSystem && s.haveFS && dir.FileSystem != s.rootFS {
			entry.skip = SkipFileSystemBoundary
		}
		if options.FollowLinks {
			id := dir.Identity
			entry.dirID = &id
		}
	}
	if entry.skip != SkipNone || !entry.isDir {
		return nil
	}
	if options.MaxDepth != nil && entry.depth >= *options.MaxDepth {
		entry.skip = SkipMaxDepth
	} else if options.FollowLinks && entry.dirID != nil && containsID(job.ancestors, *entry.dirID) {
		entry.skip = SkipSymlinkLoop
	}
	return nil
}

type dirJob struct {
	path      string
	depth     int
	ancestors []platformID
}

type platformID struct {
	fs   uint64
	file uint64
}

type dirQueue struct {
	mu       sync.Mutex
	cond     *sync.Cond
	items    []dirJob
	inflight int
	closed   bool
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
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

func (v FileVersion) Reusable(other FileVersion) bool {
	if v.ModifiedNS == nil || other.ModifiedNS == nil || *v.ModifiedNS != *other.ModifiedNS {
		return false
	}
	if v.ChangedNS != nil && other.ChangedNS != nil && *v.ChangedNS != *other.ChangedNS {
		return false
	}
	if v.Identity != nil && other.Identity != nil && !v.Identity.Equal(*other.Identity) {
		return false
	}
	return true
}

type ParallelWalkIter struct {
	ch     chan item
	token  *cancelFlag
	once   sync.Once
	closed bool
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
		ch := make(chan item)
		close(ch)
		return &ParallelWalkIter{ch: ch, closed: true}
	}
	return iter
}

func (p *ParallelWalker) TryIntoIterOrderedBounded(capacity int) (*ParallelWalkIter, error) {
	if capacity < 1 {
		capacity = 1
	}
	walker, err := NewWithOptions(p.root, p.options)
	if err != nil {
		return nil, err
	}
	ch := make(chan item, capacity)
	quit := make(chan struct{})
	flag := &cancelFlag{fn: func() { close(quit) }}
	iter := &ParallelWalkIter{ch: ch, token: flag}
	go func() {
		defer close(ch)
		defer walker.Close()
		for {
			select {
			case <-quit:
				return
			default:
			}
			entry, nextErr := walker.Next()
			if nextErr == io.EOF {
				return
			}
			it := item{err: nextErr}
			if nextErr == nil && entry != nil {
				clone := *entry
				it.entry = &clone
			}
			select {
			case <-quit:
				return
			case ch <- it:
			}
		}
	}()
	return iter, nil
}

func (p *ParallelWalker) IntoIterBounded(capacity int) *ParallelWalkIter {
	if capacity < 1 {
		capacity = 1
	}
	ch := make(chan item, capacity)
	quit := make(chan struct{})
	flag := &cancelFlag{fn: func() { close(quit) }}
	iter := &ParallelWalkIter{ch: ch, token: flag}
	go func() {
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
	}()
	return iter
}

func (it *ParallelWalkIter) Next() (*WalkEntry, error) {
	if it == nil || it.closed {
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
		it.token.cancel()
		for range it.ch {
		}
		it.closed = true
	})
	return nil
}
