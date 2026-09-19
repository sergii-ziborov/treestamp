package treestamp

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sergii-ziborov/treestamp/internal/hashx"
	"github.com/sergii-ziborov/treestamp/internal/ignore"
	pathx "github.com/sergii-ziborov/treestamp/internal/path"
	"github.com/sergii-ziborov/treestamp/internal/platform"
	"github.com/sergii-ziborov/treestamp/internal/runtime"
	"github.com/sergii-ziborov/treestamp/internal/scan"
	"github.com/sergii-ziborov/treestamp/internal/selection"
	"github.com/sergii-ziborov/treestamp/internal/walk"
	"github.com/sergii-ziborov/treestamp/internal/walkcfg"
)

type (
	ErrorPolicy          = walk.ErrorPolicy
	RootSymlinkPolicy    = walk.RootSymlinkPolicy
	WalkOptions          = walk.WalkOptions
	WalkOperation        = walk.WalkOperation
	WalkSkipReason       = walk.WalkSkipReason
	WalkEntry            = walk.WalkEntry
	WalkError            = walk.WalkError
	FileVersion          = walk.FileVersion
	Walker               = walk.Walker
	WalkBuilder          = walk.Builder
	MultiWalker          = walk.MultiWalker
	FileIdentity         = platform.Identity
	WalkControl          = walk.WalkControl
	WalkEvent            = walk.WalkEvent
	SortMode             = int
	ParallelWalkReport   = walk.ParallelWalkReport
	ParallelVisitReport  = walk.ParallelVisitReport
	ParallelWalker       = walk.ParallelWalker
	ParallelWalkIter     = walk.ParallelWalkIter
	ParallelExecutor     = runtime.Executor
	ParallelJob          = runtime.Job
	ParallelRuntime      = runtime.Runtime
	ParallelMultiWalker  = walk.MultiWalker
	WalkFunc             = walk.WalkFunc
	PathQuery            = selection.PathQuery
	SelectionMatcher     = selection.Matcher
	SelectionDecision    = selection.Decision
	SelectionDisposition = selection.Disposition
	RepositoryMatch      = ignore.Match
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
	WalkTraverseLink           = walk.WalkTraverseLink
	SortNone                   = 0
	SortLexical                = 1
	SortFilesFirst             = 2
	SortDirsFirst              = 3
	SelectedFile               = selection.SelectedFile
	TraverseDirectory          = selection.TraverseDirectory
	Unselected                 = selection.Unselected
	RepoMatchNone              = ignore.MatchNone
	RepoMatchIgnore            = ignore.MatchIgnore
	RepoMatchInclude           = ignore.MatchInclude
	RepoOverrideIgnore         = ignore.MatchOverrideIgnore
	RepoOverrideInclude        = ignore.MatchOverrideInclude
	RepoMatchHidden            = ignore.MatchHidden
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

func NewStatefulWalkBuilder[R, E any](root string, rootState R) *walk.StatefulWalkBuilder[R, E] {
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
		SkipHidden: opts.SkipHidden, StandardSkips: opts.StandardSkips, GitModules: opts.GitModules,
		Filters:       opts.Filters,
		MaxFileBytes:  opts.MaxFileBytes,
		ApplyMaxBytes: true, MinDepth: opts.Walk.MinDepth, MaxDepth: opts.Walk.MaxDepth, FollowLinks: opts.Walk.FollowLinks,
	})
}

var (
	SkipDir         = fs.SkipDir
	SkipAll         = fs.SkipAll
	SkipThis        = walk.ErrSkipThis
	ErrSkipFiles    = walk.ErrSkipFiles
	ErrAdmitTimeout = runtime.ErrAdmitTimeout
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
	Follow, Sort, ToSlash, ContentsFirst, DirsFirst bool
	FollowOutside, KeepSkipAll                      bool
	NumWorkers, MaxDepth                            int
	SortMode                                        SortMode
}

// WalkWithConfig uses serial traversal when deterministic sorting is requested.
// Sort (bool) is the legacy global-order switch and is not SortMode.
// A callback may return ErrTraverseLink to select one directory symlink.
func WalkWithConfig(root string, cfg Config, fn WalkDirFunc) error {
	inner := walkcfg.Config{
		Follow: cfg.Follow, Sort: cfg.Sort, ToSlash: cfg.ToSlash,
		ContentsFirst: cfg.ContentsFirst, DirsFirst: cfg.DirsFirst,
		NumWorkers: cfg.NumWorkers, MaxDepth: cfg.MaxDepth,
		SortMode: cfg.SortMode, FollowOutside: cfg.FollowOutside, KeepSkipAll: cfg.KeepSkipAll,
	}
	if cfg.NumWorkers == 0 || cfg.Sort {
		return walkcfg.Serial(root, inner, fn)
	}
	return walkcfg.Parallel(root, inner, fn)
}

type EntryFilter = walkcfg.EntryFilter

func NewEntryFilter() *EntryFilter { return walkcfg.NewEntryFilter() }

func IgnoreDuplicateFiles(fn WalkDirFunc) WalkDirFunc {
	return walkcfg.IgnoreDuplicateFiles(fn)
}
func IgnoreDuplicateDirs(fn WalkDirFunc) WalkDirFunc {
	return walkcfg.IgnoreDuplicateDirs(fn)
}
func IgnorePermissionErrors(fn WalkDirFunc) WalkDirFunc {
	return walkcfg.IgnorePermissionErrors(fn)
}

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
	roots                  []string
	queue                  chan *File
	exts                   []string
	ignoreGitignore        bool
	ignoreIgnoreFile       bool
	gitModules             bool
	workers                int
	maxDepth               int
	includeHidden          *bool
	ignoreBinary           bool
	binaryBytes            int
	errorHandler           func(error) bool
	walking                atomic.Bool
	terminate              atomic.Bool
	closeOnce              sync.Once
	stopOnce               sync.Once
	stop                   chan struct{}
	ctx                    context.Context
	cancel                 context.CancelFunc
	LocationExcludePattern []string
	IncludeDirectory       []string
	ExcludeDirectory       []string
	IncludeFilename        []string
	ExcludeFilename        []string
	IncludeDirectoryRegex  []*regexp.Regexp
	ExcludeDirectoryRegex  []*regexp.Regexp
	IncludeFilenameRegex   []*regexp.Regexp
	ExcludeFilenameRegex   []*regexp.Regexp
	ExcludeListExtensions  []string
	CustomIgnore           []string
	CustomIgnorePatterns   []string
}

func NewFileWalker(directory string, queue chan *File) *FileWalker {
	return newFileWalker([]string{directory}, queue, 0)
}
func NewParallelFileWalker(directory string, queue chan *File, workers int) *FileWalker {
	return newFileWalker([]string{directory}, queue, workers)
}
func newFileWalker(roots []string, queue chan *File, workers int) *FileWalker {
	ctx, cancel := context.WithCancel(context.Background())
	return &FileWalker{roots: roots, queue: queue, workers: workers, ctx: ctx, cancel: cancel, stop: make(chan struct{})}
}
func (w *FileWalker) AddRoot(directory string) *FileWalker {
	w.roots = append(w.roots, directory)
	return w
}
func (w *FileWalker) AllowListExtensions(exts ...string) *FileWalker {
	w.exts = append(w.exts[:0], exts...)
	return w
}
func (w *FileWalker) SetConcurrency(n int) *FileWalker       { w.workers = n; return w }
func (w *FileWalker) IgnoreGitignore() *FileWalker           { w.ignoreGitignore = true; return w }
func (w *FileWalker) IgnoreIgnoreFile() *FileWalker          { w.ignoreIgnoreFile = true; return w }
func (w *FileWalker) RespectGitModules() *FileWalker         { w.gitModules = true; return w }
func (w *FileWalker) IncludeHidden(enabled bool) *FileWalker { w.includeHidden = &enabled; return w }
func (w *FileWalker) IgnoreBinaryFiles() *FileWalker         { w.ignoreBinary = true; return w }
func (w *FileWalker) SetMaxDepth(n int) *FileWalker          { w.maxDepth = n; return w }
func (w *FileWalker) Terminate() {
	w.terminate.Store(true)
	if w.cancel != nil {
		w.cancel()
	}
	w.stopOnce.Do(func() { close(w.stop) })
}
func (w *FileWalker) Walking() bool                                   { return w.walking.Load() }
func (w *FileWalker) SetErrorHandler(fn func(error) bool) *FileWalker { w.errorHandler = fn; return w }
func (w *FileWalker) Start() error                                    { return runFileWalker(w) }

type MultiScanReport struct{ Reports []*ScanReport }

func (r MultiScanReport) Len() int      { return len(r.Reports) }
func (r MultiScanReport) IsEmpty() bool { return len(r.Reports) == 0 }

// Revision is one digest over the ordered roots. It does not merge file lists.
func (r MultiScanReport) Revision() string {
	h := hashx.New()
	_, _ = h.Write([]byte("multi-scan-revision\x01"))
	for _, rep := range r.Reports {
		root, rev, complete := "", "", byte(0)
		if rep != nil {
			root, rev = rep.Root, rep.Revision
			if rep.Complete {
				complete = 1
			}
		}
		_, _ = h.Write([]byte(root))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(rev))
		_, _ = h.Write([]byte{0, complete, 0xfe})
	}
	return hashx.Finish(h)
}

type MultiScanner struct {
	roots           []string
	options         Options
	rootParallelism int
	admitTimeout    time.Duration
}

func NewMultiScanner(root string) *MultiScanner {
	return &MultiScanner{roots: []string{root}, options: DefaultOptions()}
}
func (m *MultiScanner) AddRoot(root string) *MultiScanner       { m.roots = append(m.roots, root); return m }
func (m *MultiScanner) Options(options Options) *MultiScanner   { m.options = options; return m }
func (m *MultiScanner) WithRootParallelism(n int) *MultiScanner { m.rootParallelism = n; return m }
func (m *MultiScanner) WithAdmitTimeout(d time.Duration) *MultiScanner {
	m.admitTimeout = d
	return m
}
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
	timeout := m.admitTimeout
	if timeout == 0 {
		timeout = m.options.AdmitTimeout
	}
	rt := runtime.Dedicated(workers).WithAdmitTimeout(timeout)
	for i := 0; i < workers; i++ {
		if err := rt.AdmitWait(func() {
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
		}); err != nil {
			wg.Done()
			close(jobs)
			wg.Wait()
			close(results)
			return &MultiScanReport{Reports: out}, err
		}
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

func runFileWalker(w *FileWalker) error {
	w.walking.Store(true)
	defer w.walking.Store(false)
	defer w.closeOnce.Do(func() {
		if w.queue != nil {
			close(w.queue)
		}
	})
	if w.queue == nil {
		return &Error{Code: CodeInvalid, Op: "FileWalker.Start", Err: errString("file queue is nil")}
	}
	opts := fileWalkerOptions(w)
	for _, root := range w.roots {
		if w.terminate.Load() {
			return ErrTerminateWalk
		}
		if err := emitFileWalkerRoot(w, root, opts); err != nil {
			if w.errorHandler != nil && w.errorHandler(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func fileWalkerOptions(w *FileWalker) Options {
	opts := DefaultOptions().MetadataOnly()
	opts.StandardSkips = false
	if w.ignoreGitignore || w.ignoreIgnoreFile {
		opts.IgnoreFiles = dropIgnoreNames(opts.IgnoreFiles, w.ignoreGitignore, w.ignoreIgnoreFile)
	}
	if len(w.CustomIgnore) > 0 {
		opts.IgnoreFiles = append(opts.IgnoreFiles, w.CustomIgnore...)
	}
	opts.OverrideRules = append(opts.OverrideRules, w.CustomIgnorePatterns...)
	if len(w.exts) > 0 {
		opts.Extensions = append([]string(nil), w.exts...)
	}
	if w.workers > 0 {
		opts.TraversalWorkers = w.workers
	}
	if w.gitModules {
		opts.GitModules = true
	}
	if w.includeHidden != nil {
		opts.SkipHidden = !*w.includeHidden
	}
	if w.maxDepth > 0 {
		d := w.maxDepth
		opts.Walk.MaxDepth = &d
	}
	if w.ignoreBinary {
		opts.DetectBinaryFiles = true
	}
	opts.Filters = fileWalkerFilters(w)
	return opts
}

func fileWalkerFilters(w *FileWalker) Filters {
	return Filters{
		IncludeNames: w.IncludeFilename, ExcludeNames: w.ExcludeFilename,
		IncludeDirs: w.IncludeDirectory, ExcludeDirs: w.ExcludeDirectory,
		IncludeNameRegex: w.IncludeFilenameRegex, ExcludeNameRegex: w.ExcludeFilenameRegex,
		IncludeDirRegex: w.IncludeDirectoryRegex, ExcludeDirRegex: w.ExcludeDirectoryRegex,
		ExcludeExtensions: w.ExcludeListExtensions, LocationExclude: w.LocationExcludePattern,
	}
}

func dropIgnoreNames(names []string, dropGit, dropIgnore bool) []string {
	kept := names[:0]
	for _, name := range names {
		if (dropGit && name == ".gitignore") || (dropIgnore && name == ".ignore") {
			continue
		}
		kept = append(kept, name)
	}
	return kept
}

func emitFileWalkerRoot(w *FileWalker, root string, opts Options) error {
	scanner, err := NewScanner(root, WithOptions(opts))
	if err != nil {
		return err
	}
	if opts.DetectBinaryFiles {
		return emitScannedFiles(w, root, scanner)
	}
	return scan.StreamPaths(walkerContext(w), root, toScanOptions(opts), func(rel string) error {
		return pushWalkerPath(w, root, rel)
	})
}

func emitScannedFiles(w *FileWalker, root string, scanner *Scanner) error {
	report, err := scanner.Scan(walkerContext(w))
	if err != nil {
		return err
	}
	for _, file := range report.Files {
		if err := pushWalkerPath(w, root, file.Relative); err != nil {
			return err
		}
	}
	return nil
}

func walkerContext(w *FileWalker) context.Context {
	if w != nil && w.ctx != nil {
		return w.ctx
	}
	return context.Background()
}

func pushWalkerPath(w *FileWalker, root, rel string) error {
	if w.terminate.Load() {
		return ErrTerminateWalk
	}
	native := filepath.FromSlash(rel)
	file := &File{Location: filepath.Join(root, filepath.Dir(native)), Filename: filepath.Base(native)}
	if w.stop == nil {
		w.queue <- file
		return nil
	}
	select {
	case <-w.stop:
		return ErrTerminateWalk
	case w.queue <- file:
		return nil
	}
}
