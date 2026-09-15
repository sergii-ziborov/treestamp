package treestamp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sergii-ziborov/treestamp/internal/ignore"
	pathx "github.com/sergii-ziborov/treestamp/internal/path"
	"github.com/sergii-ziborov/treestamp/internal/platform"
	"github.com/sergii-ziborov/treestamp/internal/runtime"
	"github.com/sergii-ziborov/treestamp/internal/selection"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

type (
	ErrorPolicy                   = walk.ErrorPolicy
	RootSymlinkPolicy             = walk.RootSymlinkPolicy
	WalkOptions                   = walk.WalkOptions
	WalkOperation                 = walk.WalkOperation
	WalkSkipReason                = walk.WalkSkipReason
	WalkEntry                     = walk.WalkEntry
	WalkError                     = walk.WalkError
	FileVersion                   = walk.FileVersion
	Walker                        = walk.Walker
	WalkBuilder                   = walk.Builder
	MultiWalker                   = walk.MultiWalker
	FileIdentity                  = platform.Identity
	WalkControl                   = walk.WalkControl
	WalkEvent                     = walk.WalkEvent
	ParallelWalkReport            = walk.ParallelWalkReport
	ParallelVisitReport           = walk.ParallelVisitReport
	ParallelWalker                = walk.ParallelWalker
	ParallelWalkIter              = walk.ParallelWalkIter
	ParallelExecutor              = runtime.Executor
	ParallelJob                   = runtime.Job
	ParallelRuntime               = runtime.Runtime
	ParallelMultiWalker           = walk.MultiWalker
	WalkFunc                      = walk.WalkFunc
	PathQuery                     = selection.PathQuery
	SelectionMatcher              = selection.Matcher
	SelectionDecision             = selection.Decision
	SelectionDisposition          = selection.Disposition
	RepositoryMatch               = ignore.Match
	StatefulWalkEntry[E any]      = walk.StatefulWalkEntry[E]
	StatefulWalkBuilder[R, E any] = walk.StatefulWalkBuilder[R, E]
	StatefulWalker[R, E any]      = walk.StatefulWalker[R, E]
	ParallelStatefulWalker[E any] = walk.ParallelStatefulWalker[E]
	StatefulResult[E any]         = walk.StatefulResult[E]
)

const (
	ErrorContinue              = walk.ErrorContinue
	ErrorAbort                 = walk.ErrorAbort
	RootFollow                 = walk.RootFollow
	RootReject                 = walk.RootReject
	DefaultMaxOpen             = walk.DefaultMaxOpen
	OpCanonicalize             = walk.OpCanonicalize
	OpReadDirectory            = walk.OpReadDirectory
	OpReadEntry                = walk.OpReadEntry
	OpReadMetadata             = walk.OpReadMetadata
	OpScheduleWorker           = walk.OpScheduleWorker
	WalkSkipNone               = walk.SkipNone
	WalkSkipMaxDepth           = walk.SkipMaxDepth
	WalkSkipFileSystemBoundary = walk.SkipFileSystemBoundary
	WalkSkipPathEscape         = walk.SkipPathEscape
	WalkSkipSymlinkLoop        = walk.SkipSymlinkLoop
	WalkContinue               = walk.WalkContinue
	WalkSkip                   = walk.WalkSkip
	WalkQuit                   = walk.WalkQuit
	// WalkTraverseLink selects one directory symlink in ParallelWalker.Visit.
	WalkTraverseLink    = walk.WalkTraverseLink
	SelectedFile        = selection.SelectedFile
	TraverseDirectory   = selection.TraverseDirectory
	Unselected          = selection.Unselected
	RepoMatchNone       = ignore.MatchNone
	RepoMatchIgnore     = ignore.MatchIgnore
	RepoMatchInclude    = ignore.MatchInclude
	RepoOverrideIgnore  = ignore.MatchOverrideIgnore
	RepoOverrideInclude = ignore.MatchOverrideInclude
	RepoMatchHidden     = ignore.MatchHidden
)

func DefaultWalkOptions() WalkOptions        { return walk.DefaultOptions() }
func NewWalker(root string) (*Walker, error) { return walk.New(root) }
func NewWalkerWithOptions(root string, options WalkOptions) (*Walker, error) {
	return walk.NewWithOptions(root, options)
}
func NewWalkBuilder(root string) *WalkBuilder { return walk.NewBuilder(root) }
func CollectWalk(next func() (*WalkEntry, error)) ([]*WalkEntry, error) {
	return walk.Collect(adapter(next))
}

type adapter func() (*WalkEntry, error)

func (a adapter) Next() (*WalkEntry, error) { return a() }

func WalkParallel(root string, workers int, fn walk.WalkFunc) error {
	return walk.WalkParallel(root, workers, fn)
}
func CollectParallel(root string, workers int, sortPaths bool) ([]*WalkEntry, error) {
	return walk.CollectParallel(root, workers, sortPaths)
}
func NameSort(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) }

func NewParallelWalker(root string) *ParallelWalker      { return walk.NewParallelWalker(root) }
func GlobalRuntime() ParallelRuntime                     { return runtime.Global() }
func DedicatedRuntime(n int) ParallelRuntime             { return runtime.Dedicated(n) }
func OwnedRuntime(exec ParallelExecutor) ParallelRuntime { return runtime.Owned(exec) }

type ParallelMultiWalkReport struct{ Reports []ParallelWalkReport }
type ParallelMultiWalkEvent struct {
	RootIndex int
	Event     WalkEvent
}
type ParallelMultiVisitReport struct {
	Visited uint64
	Errors  []*WalkError
	Quit    bool
}

func NewParallelMultiWalker(root string) *parallelMulti {
	return &parallelMulti{roots: []string{root}}
}

type parallelMulti struct {
	roots   []string
	options WalkOptions
	workers int
}

func (p *parallelMulti) AddRoot(root string) *parallelMulti {
	p.roots = append(p.roots, root)
	return p
}
func (p *parallelMulti) Options(options WalkOptions) *parallelMulti { p.options = options; return p }
func (p *parallelMulti) WithParallelism(n int) *parallelMulti       { p.workers = n; return p }

func (p *parallelMulti) Walk() (*ParallelMultiWalkReport, error) {
	out := &ParallelMultiWalkReport{}
	for _, root := range p.roots {
		report, err := NewParallelWalker(root).Options(p.options).WithParallelism(p.workers).Walk()
		if report != nil {
			out.Reports = append(out.Reports, *report)
		}
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func (p *parallelMulti) Visit(fn func(ParallelMultiWalkEvent) WalkControl) (*ParallelMultiVisitReport, error) {
	out := &ParallelMultiVisitReport{}
	for i, root := range p.roots {
		report, err := NewParallelWalker(root).Options(p.options).WithParallelism(p.workers).Visit(func(ev WalkEvent) WalkControl {
			return fn(ParallelMultiWalkEvent{RootIndex: i, Event: ev})
		})
		if report != nil {
			out.Visited += report.Visited
			out.Errors = append(out.Errors, report.Errors...)
			out.Quit = out.Quit || report.Quit
		}
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func NewStatefulWalkBuilder[R, E any](root string, rootState R) *StatefulWalkBuilder[R, E] {
	return walk.NewStatefulWalkBuilder[R, E](root, rootState)
}

func IsSameOrDescendant(path, prefix string) bool { return pathx.IsSameOrDescendant(path, prefix) }
func PathCoveredByPrefixes(path string, prefixes []string) bool {
	return pathx.PathCoveredByPrefixes(path, prefixes)
}
func CollapsePathPrefixes(prefixes []string) []string { return pathx.CollapsePathPrefixes(prefixes) }

func NewSelectionMatcher(root string, opts Options) (*SelectionMatcher, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return selection.NewMatcher(abs, selection.Config{
		IgnoreFiles: opts.IgnoreFiles, IgnoreCase: opts.IgnoreCase, IgnorePolicy: opts.IgnorePolicy.inner,
		OverrideRules: opts.OverrideRules, Extensions: opts.Extensions, FileTypes: opts.FileTypes,
		SkipHidden: opts.SkipHidden, StandardSkips: opts.StandardSkips, MaxFileBytes: opts.MaxFileBytes,
		ApplyMaxBytes: true, MinDepth: opts.Walk.MinDepth, MaxDepth: opts.Walk.MaxDepth, FollowLinks: opts.Walk.FollowLinks,
	})
}

var (
	SkipDir      = fs.SkipDir
	SkipAll      = fs.SkipAll
	ErrSkipFiles = errors.New("skip remaining files in this directory")
	// ErrTraverseLink requests traversal of the callback's directory symlink.
	ErrTraverseLink = walk.ErrTraverseLink
)

type WalkDirFunc = fs.WalkDirFunc

func Walk(root string, fn WalkDirFunc) error        { return WalkWithConfig(root, Config{}, fn) }
func WalkDefault(root string, fn WalkDirFunc) error { return Walk(root, fn) }
func WalkUnsorted(root string, fn WalkDirFunc) error {
	return WalkWithConfig(root, Config{NumWorkers: -1}, fn)
}

type Config struct {
	Follow     bool
	Sort       bool
	NumWorkers int
	MaxDepth   int
	ToSlash    bool
}

// WalkWithConfig uses serial traversal when deterministic sorting is requested.
// A callback may return ErrTraverseLink to select one directory symlink.
func WalkWithConfig(root string, cfg Config, fn WalkDirFunc) error {
	if cfg.NumWorkers == 0 || cfg.Sort {
		return walkSerialConfig(root, cfg, fn)
	}
	return walkParallelConfig(root, cfg, fn)
}

func walkSerialConfig(root string, cfg Config, fn WalkDirFunc) error {
	if !cfg.Follow && cfg.MaxDepth == 0 && !cfg.Sort {
		walker, err := NewWalker(root)
		if err != nil {
			return fn(root, nil, err)
		}
		defer walker.Close()
		return drainWalk(walker, fn, cfg.ToSlash)
	}
	opts := DefaultWalkOptions()
	opts.FollowLinks = cfg.Follow
	if cfg.MaxDepth > 0 {
		d := cfg.MaxDepth
		opts.MaxDepth = &d
	}
	builder := NewWalkBuilder(root).Options(opts)
	if cfg.Sort {
		builder = builder.SortByFileName()
	}
	walker := builder.Build()
	defer walker.Close()
	return drainWalk(walker, fn, cfg.ToSlash)
}

func walkParallelConfig(root string, cfg Config, fn WalkDirFunc) error {
	workers := cfg.NumWorkers
	if workers < 0 {
		workers = 0
	}
	opts := DefaultWalkOptions()
	opts.FollowLinks = cfg.Follow
	if cfg.MaxDepth > 0 {
		d := cfg.MaxDepth
		opts.MaxDepth = &d
	}
	var mu sync.Mutex
	var callbackErr error
	skipFiles := map[string]struct{}{}
	_, err := NewParallelWalker(root).Options(opts).WithParallelism(workers).Visit(func(ev WalkEvent) WalkControl {
		return applyWalkCallback(ev, cfg, fn, &mu, skipFiles, &callbackErr)
	})
	if callbackErr != nil {
		return callbackErr
	}
	return err
}

func applyWalkCallback(ev WalkEvent, cfg Config, fn WalkDirFunc, mu *sync.Mutex, skipFiles map[string]struct{}, callbackErr *error) WalkControl {
	if ev.Err != nil {
		return walkCallbackControl(fn(ev.Err.Path, nil, ev.Err), mu, skipFiles, filepath.Dir(ev.Err.Path), callbackErr)
	}
	path := ev.Entry.Path()
	if cfg.ToSlash {
		path = filepath.ToSlash(path)
	}
	info, err := os.Lstat(ev.Entry.Path())
	if err != nil {
		return walkCallbackControl(fn(path, nil, err), mu, skipFiles, filepath.Dir(ev.Entry.Path()), callbackErr)
	}
	parent := filepath.Dir(ev.Entry.Path())
	mu.Lock()
	_, skip := skipFiles[parent]
	mu.Unlock()
	if skip && info.Mode().IsRegular() {
		return WalkContinue
	}
	cbErr := fn(path, walk.NewDirEntry(ev.Entry, info), nil)
	if errors.Is(cbErr, ErrTraverseLink) && info.Mode()&os.ModeSymlink != 0 {
		return walk.WalkTraverseLink
	}
	return walkCallbackControl(cbErr, mu, skipFiles, parent, callbackErr)
}

func walkCallbackControl(cbErr error, mu *sync.Mutex, skipFiles map[string]struct{}, parent string, callbackErr *error) WalkControl {
	switch {
	case errors.Is(cbErr, SkipDir):
		return WalkSkip
	case errors.Is(cbErr, SkipAll):
		return WalkQuit
	case errors.Is(cbErr, ErrSkipFiles):
		mu.Lock()
		skipFiles[parent] = struct{}{}
		mu.Unlock()
		return WalkContinue
	case cbErr == nil:
		return WalkContinue
	default:
		mu.Lock()
		if *callbackErr == nil {
			*callbackErr = cbErr
		}
		mu.Unlock()
		return WalkQuit
	}
}

func drainWalk(w interface {
	Next() (*WalkEntry, error)
	Close() error
}, fn WalkDirFunc, toSlash bool) error {
	var skipFiles bool
	var skipDir string
	for {
		entry, err := w.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fn("", nil, err)
		}
		path := entry.Path()
		if toSlash {
			path = filepath.ToSlash(path)
		}
		info, statErr := os.Lstat(entry.Path())
		if statErr != nil {
			if fn(path, nil, statErr) != nil {
				return statErr
			}
			continue
		}
		parent := filepath.Dir(entry.Path())
		if parent != skipDir {
			skipFiles, skipDir = false, parent
		}
		if skipFiles && info.Mode().IsRegular() {
			continue
		}
		switch cbErr := fn(path, walk.NewDirEntry(entry, info), nil); {
		case errors.Is(cbErr, SkipDir):
			if skipper, ok := w.(interface{ SkipCurrentDir() }); ok {
				skipper.SkipCurrentDir()
			}
		case errors.Is(cbErr, SkipAll):
			return nil
		case errors.Is(cbErr, ErrSkipFiles):
			skipFiles = true
		case errors.Is(cbErr, ErrTraverseLink) && info.Mode()&os.ModeSymlink != 0:
			follower, ok := w.(interface{ TraverseCurrentSymlink() error })
			if !ok {
				return cbErr
			}
			if err := follower.TraverseCurrentSymlink(); err != nil {
				return err
			}
		case cbErr == nil:
		default:
			return cbErr
		}
	}
}

func IgnoreDuplicateFiles(fn WalkDirFunc) WalkDirFunc { return ignoreDuplicate(fn, false) }
func IgnoreDuplicateDirs(fn WalkDirFunc) WalkDirFunc  { return ignoreDuplicate(fn, true) }

func ignoreDuplicate(fn WalkDirFunc, dirsOnly bool) WalkDirFunc {
	seen := map[string]struct{}{}
	var mu sync.Mutex
	return func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || (dirsOnly && !d.IsDir()) {
			return fn(path, d, err)
		}
		key := filepath.Clean(path)
		if !dirsOnly {
			if info, infoErr := d.Info(); infoErr == nil {
				key = key + "\x00" + info.ModTime().String() + "\x00" + strconv.FormatUint(uint64(info.Size()), 10)
			}
		}
		mu.Lock()
		_, dup := seen[key]
		if !dup {
			seen[key] = struct{}{}
		}
		mu.Unlock()
		if dup {
			if d.IsDir() {
				return SkipDir
			}
			return nil
		}
		return fn(path, d, err)
	}
}

func ReadDirents(dirname string) ([]os.DirEntry, error) { return os.ReadDir(dirname) }

type File struct {
	Location string
	Filename string
}

func (f *File) Path() string {
	if f == nil {
		return ""
	}
	if f.Location == "" {
		return f.Filename
	}
	return filepath.Join(f.Location, f.Filename)
}

type FileWalker struct {
	roots           []string
	queue           chan *File
	exts            []string
	ignoreGitignore bool
	workers         int
	terminate       atomic.Bool
	closeOnce       sync.Once
}

func NewFileWalker(directory string, queue chan *File) *FileWalker {
	return &FileWalker{roots: []string{directory}, queue: queue}
}
func NewParallelFileWalker(directory string, queue chan *File, workers int) *FileWalker {
	return &FileWalker{roots: []string{directory}, queue: queue, workers: workers}
}
func (w *FileWalker) AddRoot(directory string) *FileWalker {
	w.roots = append(w.roots, directory)
	return w
}
func (w *FileWalker) AllowListExtensions(exts ...string) *FileWalker {
	w.exts = append(w.exts[:0], exts...)
	return w
}
func (w *FileWalker) SetConcurrency(n int) *FileWalker { w.workers = n; return w }
func (w *FileWalker) IgnoreGitignore() *FileWalker     { w.ignoreGitignore = true; return w }
func (w *FileWalker) Terminate()                       { w.terminate.Store(true) }

func (w *FileWalker) Start() error {
	defer w.closeOnce.Do(func() {
		if w.queue != nil {
			close(w.queue)
		}
	})
	if w.queue == nil {
		return &Error{Code: CodeInvalid, Op: "FileWalker.Start", Err: errString("file queue is nil")}
	}
	opts := DefaultOptions().MetadataOnly()
	if w.ignoreGitignore {
		kept := opts.IgnoreFiles[:0]
		for _, name := range opts.IgnoreFiles {
			if name != ".gitignore" {
				kept = append(kept, name)
			}
		}
		opts.IgnoreFiles = kept
	}
	if len(w.exts) > 0 {
		opts.Extensions = append([]string(nil), w.exts...)
	}
	if w.workers > 0 {
		opts.TraversalWorkers = w.workers
	}
	for _, root := range w.roots {
		if w.terminate.Load() {
			return nil
		}
		scanner, err := NewScanner(root, WithOptions(opts))
		if err != nil {
			return err
		}
		paths, err := scanner.ScanPaths(context.Background())
		if err != nil {
			return err
		}
		for _, rel := range paths {
			if w.terminate.Load() {
				return nil
			}
			native := filepath.FromSlash(rel)
			w.queue <- &File{Location: filepath.Join(root, filepath.Dir(native)), Filename: filepath.Base(native)}
		}
	}
	return nil
}

type MultiScanReport struct{ Reports []*ScanReport }

func (r MultiScanReport) Len() int      { return len(r.Reports) }
func (r MultiScanReport) IsEmpty() bool { return len(r.Reports) == 0 }

type MultiScanner struct {
	roots           []string
	options         Options
	rootParallelism int
}

func NewMultiScanner(root string) *MultiScanner {
	return &MultiScanner{roots: []string{root}, options: DefaultOptions()}
}
func (m *MultiScanner) AddRoot(root string) *MultiScanner       { m.roots = append(m.roots, root); return m }
func (m *MultiScanner) Options(options Options) *MultiScanner   { m.options = options; return m }
func (m *MultiScanner) WithRootParallelism(n int) *MultiScanner { m.rootParallelism = n; return m }

func (m *MultiScanner) Scan(ctx context.Context) (*MultiScanReport, error) {
	workers := m.rootParallelism
	if workers <= 0 {
		workers = goruntime.GOMAXPROCS(0)
		if workers > 8 {
			workers = 8
		}
	}
	if workers > len(m.roots) {
		workers = len(m.roots)
	}
	if workers < 1 {
		workers = 1
	}
	type item struct {
		i      int
		report *ScanReport
		err    error
	}
	out := make([]*ScanReport, len(m.roots))
	jobs, results := make(chan int), make(chan item, len(m.roots))
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for idx := range jobs {
				scanner, err := NewScanner(m.roots[idx], WithOptions(m.options))
				if err != nil {
					results <- item{i: idx, err: err}
					continue
				}
				report, err := scanner.Scan(ctx)
				results <- item{i: idx, report: report, err: err}
			}
		}()
	}
	go func() {
		for i := range m.roots {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	var first error
	for item := range results {
		if item.err != nil && first == nil {
			first = item.err
		}
		out[item.i] = item.report
	}
	return &MultiScanReport{Reports: out}, first
}

func (m *MultiScanner) VisitContent(ctx context.Context, factory func(root, worker int) ContentVisitor) (*MultiContentVisitReport, error) {
	out := &MultiContentVisitReport{}
	for i, root := range m.roots {
		scanner, err := NewScanner(root, WithOptions(m.options))
		if err != nil {
			return out, err
		}
		report, err := scanner.VisitContent(ctx, func(worker int) ContentVisitor { return factory(i, worker) })
		if err != nil {
			return out, err
		}
		out.Reports = append(out.Reports, *report)
	}
	return out, nil
}
