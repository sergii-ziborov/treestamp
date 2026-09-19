package walk

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/sergii-ziborov/treestamp/internal/dirread"
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
		// Native cap is 8 on every OS. This is not the fastwalk Darwin table
		// and not a Linux getdents claim for Darwin or Windows.
		if n = p.runtime.Parallelism(); n > 8 {
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

func (p *ParallelWalker) Visit(fn ControlFunc) (*ParallelVisitReport, error) {
	return p.VisitWithToken(nil, fn)
}

func (p *ParallelWalker) VisitWithToken(token *rtruntime.Token, fn ControlFunc) (*ParallelVisitReport, error) {
	return walkConcurrent(p.root, concurrentOpts{
		options: p.options.Normalize(), workers: p.workerCount(), skipStdout: p.skipStdout,
		token: token, runtime: p.runtime,
	}, fn)
}

type concurrentOpts struct {
	options    WalkOptions
	workers    int
	skipStdout bool
	token      *rtruntime.Token
	runtime    rtruntime.Runtime
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
	control := emit(WalkEvent{Entry: state.rootEntry})
	if control == WalkQuit || control == WalkSkip || !state.rootEntry.isDir || state.rootEntry.hasSkip() {
		return state.report, state.first
	}
	queue := newDirQueue()
	var ancestors []platformID
	if state.rootEntry.dirID != nil {
		ancestors = []platformID{{fs: state.rootEntry.dirID.FileSystem, file: state.rootEntry.dirID.File}}
	}
	queue.push(dirJob{path: state.abs, depth: 0, ancestors: ancestors})
	var wg sync.WaitGroup
	started := 0
	for i := 0; i < opts.workers; i++ {
		wg.Add(1)
		if err := opts.runtime.AdmitWait(func() {
			defer wg.Done()
			for !state.quit.Load() {
				job, ok := queue.pop()
				if !ok {
					return
				}
				state.processJob(job, queue, emit)
			}
			queue.close()
		}); err != nil {
			wg.Done()
			state.mu.Lock()
			if state.first == nil {
				state.first = err
			}
			state.mu.Unlock()
			state.quit.Store(true)
			queue.close()
			break
		}
		started++
	}
	if started == 0 {
		queue.close()
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
	state.rootEntry = makeEntry(abs, abs, 0, info, opts.options, nil)
	if state.rootEntry.isDir && opts.options.atOrBeyondMaxDepth(0) {
		state.rootEntry.skip = SkipMaxDepth
	}
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
		resolved, resolveErr := pathx.Resolve(abs)
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
			s.mu.Lock()
			s.report.Cancelled = true
			s.mu.Unlock()
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
	entries, readErr := dirread.OSEntries(job.path)
	if readErr != nil {
		emit(WalkEvent{Err: walkErr(job.path, job.depth+1, OpReadDirectory, readErr)})
		queue.done()
		return
	}
	for _, dent := range orderDirents(entries, s.opts.options) {
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
	path := childPath(job.path, dent.Name())
	entry, err := s.entryFromDent(job, dent, path)
	if err != nil {
		return emit(WalkEvent{Err: err})
	}
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
		canonical, err := pathx.Resolve(path)
		if err != nil {
			if pathx.EscapesRoot(s.abs, path) {
				entry.skip = SkipPathEscape
				return nil
			}
			return walkErr(path, entry.depth, OpCanonicalize, err)
		}
		if !options.FollowOutside && !pathx.UnderRoot(s.abs, canonical) {
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

func (p *ParallelWalker) TryIntoIterOrderedBounded(capacity int) (*ParallelWalkIter, error) {
	if capacity < 1 {
		capacity = 1
	}
	opts := concurrentOpts{
		options: p.options.Normalize(), workers: p.workerCount(),
		skipStdout: p.skipStdout, runtime: p.runtime,
	}
	state, err := setupConcurrent(p.root, opts)
	if err != nil {
		return nil, err
	}
	ch := make(chan item, capacity)
	quit := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	pull, err := startOrdered(state, opts, capacity, ctx, quit)
	if err != nil {
		cancel()
		return nil, err
	}
	iter := &ParallelWalkIter{ch: ch, token: &cancelFlag{fn: func() { close(quit); cancel() }}}
	go func() {
		defer close(ch)
		defer cancel()
		defer pull.stop()
		emitOrdered(state, pull, ch)
	}()
	return iter, nil
}

type listedDir struct {
	entries  []*WalkEntry
	descents []dirJob
	err      error
	bytes    int64
}

type orderedPull struct {
	state  *concurrentState
	opts   concurrentOpts
	budget *rtruntime.Budget
	ctx    context.Context
	quit   <-chan struct{}
	mu     sync.Mutex
	cond   *sync.Cond
	listed map[string]*listedDir
	jobs   map[string]dirJob
	queue  []dirJob
	queued map[string]bool
	window int
	stopCh chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
}

func startOrdered(state *concurrentState, opts concurrentOpts, window int, ctx context.Context, quit <-chan struct{}) (*orderedPull, error) {
	lim := rtruntime.DefaultLimits(opts.workers)
	lim.Ready = window
	p := &orderedPull{
		state: state, opts: opts, budget: rtruntime.NewBudget(lim), ctx: ctx, quit: quit,
		listed: map[string]*listedDir{}, jobs: map[string]dirJob{}, queued: map[string]bool{},
		window: window, stopCh: make(chan struct{}),
	}
	p.cond = sync.NewCond(&p.mu)
	var first error
	n := 0
	for i := 0; i < opts.workers; i++ {
		p.wg.Add(1)
		if err := opts.runtime.AdmitWait(p.worker); err != nil {
			p.wg.Done()
			if first == nil {
				first = err
			}
			continue
		}
		n++
	}
	if n == 0 {
		p.stop()
		if first == nil {
			first = rtruntime.ErrBusy
		}
		return nil, first
	}
	context.AfterFunc(ctx, p.wake)
	return p, nil
}

func (p *orderedPull) wake() { p.mu.Lock(); p.cond.Broadcast(); p.mu.Unlock() }

func (p *orderedPull) worker() {
	defer p.wg.Done()
	for {
		job, ok := p.take()
		if !ok {
			return
		}
		p.publish(job.path, p.work(job))
	}
}

func (p *orderedPull) work(job dirJob) *listedDir {
	sched := func(err error) *listedDir {
		return &listedDir{err: walkErr(job.path, job.depth+1, OpScheduleWorker, err)}
	}
	if err := p.budget.Admit(p.ctx, rtruntime.KindDirectory); err != nil {
		return sched(err)
	}
	listed := p.list(job)
	if listed.err == nil {
		listed.bytes = listedBytes(listed)
		if err := p.budget.HoldReady(p.ctx, listed.bytes); err != nil {
			listed = sched(err)
		}
	}
	p.budget.Release(rtruntime.KindDirectory)
	return listed
}

func (p *orderedPull) list(job dirJob) *listedDir {
	dents, err := dirread.OSEntries(job.path)
	if err != nil {
		return &listedDir{err: walkErr(job.path, job.depth+1, OpReadDirectory, err)}
	}
	out := &listedDir{}
	for _, dent := range orderDirents(dents, p.opts.options) {
		entry, next, listErr := p.child(job, dent)
		if listErr != nil {
			return &listedDir{err: listErr}
		}
		if entry == nil {
			continue
		}
		out.entries = append(out.entries, entry)
		if next != nil {
			out.descents = append(out.descents, *next)
		}
	}
	return out
}

func (p *orderedPull) child(job dirJob, dent os.DirEntry) (*WalkEntry, *dirJob, *WalkError) {
	path := childPath(job.path, dent.Name())
	entry, err := p.state.entryFromDent(job, dent, path)
	if err != nil {
		return nil, nil, err
	}
	if p.state.stdout != nil && entry.isFile {
		if match, _ := platform.PathMatchesIdentity(path, *p.state.stdout); match {
			return nil, nil, nil
		}
	}
	if policyErr := p.state.applyDirPolicy(entry, path, job); policyErr != nil {
		return nil, nil, policyErr
	}
	if !entry.isDir || entry.hasSkip() {
		return entry, nil, nil
	}
	next := dirJob{path: path, depth: entry.depth, ancestors: job.ancestors}
	if entry.dirID != nil {
		next.ancestors = append(append([]platformID(nil), job.ancestors...), platformID{fs: entry.dirID.FileSystem, file: entry.dirID.File})
	}
	return entry, &next, nil
}

func (p *orderedPull) emitDir(path string, ch chan item) bool {
	got := p.waitListed(path)
	ok := got != nil
	if got != nil && got.err != nil {
		ok = sendOrdered(p.quit, ch, item{err: got.err})
	} else if got != nil {
		for _, entry := range got.entries {
			clone := *entry
			if !sendOrdered(p.quit, ch, item{entry: &clone}) || (entry.isDir && !entry.hasSkip() && !p.emitDir(entry.path, ch)) {
				ok = false
				break
			}
		}
	}
	p.forget(path)
	return ok
}

func (p *orderedPull) addJob(job dirJob, force bool) {
	if job.path == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.jobs[job.path] = job
	if p.queued[job.path] || p.listed[job.path] != nil || (!force && p.window > 0 && len(p.queue) >= p.window) {
		return
	}
	p.queued[job.path] = true
	p.queue = append(p.queue, job)
	p.cond.Signal()
}

func (p *orderedPull) take() (dirJob, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for !p.done() {
		if len(p.queue) > 0 {
			job := p.queue[0]
			p.queue = p.queue[1:]
			return job, true
		}
		p.cond.Wait()
	}
	return dirJob{}, false
}

func (p *orderedPull) publish(path string, listed *listedDir) {
	p.mu.Lock()
	p.listed[path] = listed
	p.cond.Broadcast()
	p.mu.Unlock()
	if listed == nil {
		return
	}
	for _, job := range listed.descents {
		p.addJob(job, false)
	}
}

func (p *orderedPull) waitListed(path string) *listedDir {
	p.mu.Lock()
	if job, ok := p.jobs[path]; ok {
		p.mu.Unlock()
		p.addJob(job, true)
		p.mu.Lock()
	}
	defer p.mu.Unlock()
	for !p.done() {
		if got := p.listed[path]; got != nil {
			return got
		}
		p.cond.Wait()
	}
	return nil
}

func (p *orderedPull) stop() {
	p.once.Do(func() { close(p.stopCh) })
	p.wake()
	p.budget.Close()
	p.wg.Wait()
}

func (p *orderedPull) done() bool {
	select {
	case <-p.stopCh:
		return true
	case <-p.quit:
		return true
	case <-p.ctx.Done():
		return true
	default:
		return false
	}
}
