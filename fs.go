// Package treestamp is a native Go library for deterministic repository scanning.
//
// It selects files under ignore and filter rules, hashes selected content,
// and explains why a path was kept or dropped. Start with [ScanWith],
// [ScanPathsWith], [EachFile], or [Compile]. [Explain] is selection only;
// it does not re-read file bytes.
//
// Requires Go 1.21 or newer. The command-line app is the nested module
// github.com/sergii-ziborov/treestamp/cmd/treestamp.
package treestamp

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sergii-ziborov/treestamp/internal/dirread"
	"github.com/sergii-ziborov/treestamp/internal/dx"
	"github.com/sergii-ziborov/treestamp/internal/runtime"
	"github.com/sergii-ziborov/treestamp/internal/selection"
	"github.com/sergii-ziborov/treestamp/internal/walk"
	"github.com/sergii-ziborov/treestamp/internal/walkfs"
)

type (
	DirEntry = walk.DirEntry
	FSEntry  = walkfs.Entry
	FSWalker = walkfs.Walker
)

// StatDirEntry returns cached os.Stat metadata when entry supports it.
func StatDirEntry(path string, entry fs.DirEntry) (fs.FileInfo, error) {
	return walk.StatDirEntry(path, entry)
}

// DirEntryDepth returns callback depth or -1 for another DirEntry type.
func DirEntryDepth(entry fs.DirEntry) int {
	return walk.DirEntryDepth(entry)
}

// NewFSWalker constructs a deterministic pull walker over an arbitrary fs.FS.
func NewFSWalker(fsys fs.FS, root string) (*FSWalker, error) {
	return walkfs.New(fsys, root)
}

// WalkFS walks an arbitrary fs.FS with fs.WalkDir-compatible callback control.
func WalkFS(fsys fs.FS, root string, fn fs.WalkDirFunc) error {
	return walkfs.Walk(fsys, root, fn)
}

// ScanFS is ScanWith over an fs.FS. Walk is lexical and does not follow
// symlinks. Native identity, ConfineAt, and cache reuse do not apply.
func ScanFS(ctx context.Context, fsys fs.FS, root string, opts ...Option) (*ScanReport, error) {
	p, err := Compile(opts...)
	if err != nil {
		return nil, err
	}
	return p.ScanFS(ctx, fsys, root)
}

// ScanPathsFS is ScanPathsWith over an fs.FS. Content options are rejected.
func ScanPathsFS(ctx context.Context, fsys fs.FS, root string, opts ...Option) ([]string, error) {
	p, err := Compile(opts...)
	if err != nil {
		return nil, err
	}
	return p.ScanPathsFS(ctx, fsys, root)
}

// EachFileFS is EachFile over an fs.FS. A callback error is not replayed.
func EachFileFS(ctx context.Context, fsys fs.FS, root string, consume func(ScannedFile, []byte) error, opts ...Option) (*ScanSummary, error) {
	p, err := Compile(opts...)
	if err != nil {
		return nil, err
	}
	return p.EachFileFS(ctx, fsys, root, consume)
}

// MinimumScratchBufferSize is the reusable getdents buffer floor.
// It is the page size on Unix and 0 on Windows.
func MinimumScratchBufferSize() int { return dirread.MinimumScratch() }

// NewScratchBuffer allocates a reusable directory-read buffer.
func NewScratchBuffer() []byte { return dirread.EnsureScratch(nil) }

// ReadDirents lists one directory. Prefer ReadDirentsScratch in a loop.
func ReadDirents(dirname string) ([]os.DirEntry, error) {
	return ReadDirentsScratch(dirname, nil)
}

// ReadDirentsScratch lists one directory using an optional scratch buffer.
func ReadDirentsScratch(dirname string, scratch []byte) ([]os.DirEntry, error) {
	return dirread.OSEntriesScratch(dirname, scratch)
}

// ReadDirnames lists child names using an optional scratch buffer.
func ReadDirnames(dirname string, scratch []byte) ([]string, error) {
	return dirread.Names(dirname, scratch)
}

// DirScanner yields one child at a time from a single directory.
// It is not the repository Scanner.
type DirScanner struct{ inner *dirread.Scanner }

// NewDirScanner opens a lazy single-directory scanner.
func NewDirScanner(dirname string) (*DirScanner, error) {
	return NewDirScannerScratch(dirname, nil)
}

// NewDirScannerScratch opens a lazy scanner with a reusable scratch buffer.
func NewDirScannerScratch(dirname string, scratch []byte) (*DirScanner, error) {
	inner, err := dirread.NewScanner(dirname, scratch)
	if err != nil {
		return nil, err
	}
	return &DirScanner{inner: inner}, nil
}

func (s *DirScanner) Scan() bool { return s != nil && s.inner != nil && s.inner.Scan() }

func (s *DirScanner) Name() string {
	if s == nil || s.inner == nil {
		return ""
	}
	return s.inner.Name()
}

func (s *DirScanner) Dirent() (os.DirEntry, error) {
	if s == nil || s.inner == nil {
		return nil, os.ErrClosed
	}
	return s.inner.Dent(), nil
}

func (s *DirScanner) Err() error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.Err()
}

func (s *DirScanner) Close() error { return s.Err() }

// ErrTerminateWalk is returned when FileWalker.Terminate stops a walk.
var ErrTerminateWalk = errors.New("treestamp terminated")

var errPostChildrenUnsupported = errors.New("PostChildrenCallback requires NumWorkers=0")

// Filters is the public declarative name/dir/regex filter set.
type Filters = selection.Filters

// FindRepositoryRoot walks up from start looking for .git or .hg.
// If none is found it returns the absolute start directory.
func FindRepositoryRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return start
	}
	origin := dir
	for {
		if isVcsRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return origin
		}
		dir = parent
	}
}

func isVcsRoot(dir string) bool {
	if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return info.IsDir() || info.Mode().IsRegular()
	}
	info, err := os.Stat(filepath.Join(dir, ".hg"))
	return err == nil && info.IsDir()
}

// DirWalkOptions is the godirwalk-shaped callback surface.
type DirWalkOptions struct {
	Callback             WalkDirFunc
	PostChildrenCallback WalkDirFunc
	ErrorCallback        func(path string, err error) error
	Unsorted             bool
	FollowSymbolicLinks  bool
	ContentsFirst        bool
	DirsFirst            bool
	AllowNonDirectory    bool
	NumWorkers           int
	ToSlash              bool
	ScratchBuffer        []byte
	MaxOpen              int
	Context              context.Context
}

func WalkDirs(root string, opts DirWalkOptions) error {
	fn := dirWalkCallback(opts)
	if fn == nil {
		fn = func(string, fs.DirEntry, error) error { return nil }
	}
	if opts.NumWorkers != 0 {
		if opts.PostChildrenCallback != nil {
			return errPostChildrenUnsupported
		}
		return WalkWithConfig(root, Config{
			Follow: opts.FollowSymbolicLinks, ToSlash: opts.ToSlash,
			ContentsFirst: opts.ContentsFirst, DirsFirst: opts.DirsFirst,
			NumWorkers: opts.NumWorkers,
		}, fn)
	}
	return walk.WalkCallbackHooks(root, fn, walk.CallbackOptions{
		After: dirWalkAfter(opts), ToSlash: opts.ToSlash, ContentsFirst: opts.ContentsFirst,
		Sort: !opts.Unsorted, Follow: opts.FollowSymbolicLinks, Scratch: opts.ScratchBuffer,
		RequireDirectory: !opts.AllowNonDirectory, MaxOpen: opts.MaxOpen, Context: opts.Context,
	})
}

func dirWalkCallback(opts DirWalkOptions) WalkDirFunc {
	fn := opts.Callback
	if opts.ErrorCallback == nil {
		if fn == nil {
			return nil
		}
		return func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return fn(path, d, nil)
		}
	}
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return applyWalkError(opts.ErrorCallback, path, err)
		}
		if fn == nil {
			return nil
		}
		return applyWalkError(opts.ErrorCallback, path, fn(path, d, nil))
	}
}

func dirWalkAfter(opts DirWalkOptions) WalkDirFunc {
	fn := opts.PostChildrenCallback
	if fn == nil {
		return nil
	}
	if opts.ErrorCallback == nil {
		return func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return fn(path, d, nil)
		}
	}
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return applyWalkError(opts.ErrorCallback, path, err)
		}
		return applyWalkError(opts.ErrorCallback, path, fn(path, d, nil))
	}
}

func applyWalkError(policy func(string, error) error, path string, err error) error {
	if err == nil || isWalkControl(err) {
		return err
	}
	if policy == nil {
		return err
	}
	if next := policy(path, err); next == nil {
		return nil
	} else {
		return next
	}
}

func isWalkControl(err error) bool {
	return errors.Is(err, fs.SkipDir) || errors.Is(err, fs.SkipAll) || errors.Is(err, SkipThis) ||
		errors.Is(err, ErrSkipFiles) || errors.Is(err, ErrTraverseLink)
}

func CompileFilters(includeNames, excludeNames, includeDirs, excludeDirs []string, includeNameRE, excludeNameRE, includeDirRE, excludeDirRE []string) (Filters, error) {
	includeNameRegex, err := compileRegexes(includeNameRE)
	if err != nil {
		return Filters{}, err
	}
	excludeNameRegex, err := compileRegexes(excludeNameRE)
	if err != nil {
		return Filters{}, err
	}
	includeDirRegex, err := compileRegexes(includeDirRE)
	if err != nil {
		return Filters{}, err
	}
	excludeDirRegex, err := compileRegexes(excludeDirRE)
	if err != nil {
		return Filters{}, err
	}
	return Filters{
		IncludeNames: includeNames, ExcludeNames: excludeNames,
		IncludeDirs: includeDirs, ExcludeDirs: excludeDirs,
		IncludeNameRegex: includeNameRegex, ExcludeNameRegex: excludeNameRegex,
		IncludeDirRegex: includeDirRegex, ExcludeDirRegex: excludeDirRegex,
	}, nil
}

func compileRegexes(patterns []string) ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		expr, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		out = append(out, expr)
	}
	return out, nil
}

func (m *RepositoryMatcher) Match(relative string, isDir bool) RepositoryMatch {
	return m.engine.Match(relative, isDir)
}
func (m *RepositoryMatcher) EnterDir(abs, rel string) []string { return m.engine.LoadDir(abs, rel) }
func (m *RepositoryMatcher) Sources() []IgnoreSourceEvidence {
	var out []IgnoreSourceEvidence
	for _, src := range m.engine.Sources() {
		out = append(out, IgnoreSourceEvidence{Kind: src.Kind, Location: src.Location, ContentHash: src.ContentHash})
	}
	return out
}

func classifyCode(err error) ErrorCode {
	if errors.Is(err, runtime.ErrBusy) || errors.Is(err, runtime.ErrAdmitTimeout) {
		return CodeAdmission
	}
	return ErrorCode(dx.Classify(err))
}

type (
	Progress  = dx.Progress
	ByteLimit = dx.ByteLimit
)

func LimitBytes(n uint64) ByteLimit { return dx.LimitBytes(n) }
func ZeroBytes() ByteLimit          { return dx.ZeroBytes() }
func UnlimitedBytes() ByteLimit     { return dx.UnlimitedBytes() }
func SafeCause(err error) string    { return dx.SafeCause(err) }

// ErrPartial means selected work was incomplete. ErrStop is a deliberate EachFile halt.
var (
	ErrPartial = dx.ErrPartial
	ErrStop    = dx.ErrStop
)

// Option configures a compiled Plan. Scan and ScanPaths keep two-argument signatures.
type Option func(*planBuilder) error

type planBuilder struct {
	opts                                 Options
	log                                  *slog.Logger
	progress                             func(Progress)
	hashSet, binarySet, maxSet, failFast bool
	requireCache                         bool
}

// Plan is an immutable compiled scan configuration. It does not hold Context.
type Plan struct {
	opts                                 Options
	log                                  *slog.Logger
	progress                             func(Progress)
	hashSet, binarySet, maxSet, failFast bool
	requireCache                         bool
}

func Compile(opts ...Option) (*Plan, error) {
	b := planBuilder{opts: DefaultOptions()}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&b); err != nil {
			return nil, &Error{Code: CodeInvalid, Op: "Compile", Err: err}
		}
	}
	if b.opts.TraversalWorkers < 0 || b.opts.ContentWorkers < 0 {
		return nil, &Error{Code: CodeInvalid, Op: "Compile", Err: errString("negative worker count")}
	}
	if err := b.opts.Filters.Err(); err != nil {
		return nil, &Error{Code: CodeInvalid, Op: "Compile", Err: err}
	}
	b.opts = cloneOptions(b.opts)
	return &Plan{opts: b.opts, log: b.log, progress: b.progress, hashSet: b.hashSet, binarySet: b.binarySet, maxSet: b.maxSet, failFast: b.failFast, requireCache: b.requireCache}, nil
}

func ScanWith(ctx context.Context, root string, opts ...Option) (*ScanReport, error) {
	p, err := Compile(opts...)
	if err != nil {
		return nil, err
	}
	return p.Scan(ctx, root)
}

func ScanPathsWith(ctx context.Context, root string, opts ...Option) ([]string, error) {
	p, err := Compile(opts...)
	if err != nil {
		return nil, err
	}
	return p.ScanPaths(ctx, root)
}

func EachFile(ctx context.Context, root string, consume func(ScannedFile, []byte) error, opts ...Option) (*ScanSummary, error) {
	p, err := Compile(opts...)
	if err != nil {
		return nil, err
	}
	return p.EachFile(ctx, root, consume)
}

func Explain(root, relative string, opts ...Option) (PathExplanation, error) {
	p, err := Compile(opts...)
	if err != nil {
		return PathExplanation{}, err
	}
	return p.Explain(root, relative)
}

func Using(opts Options) Option {
	return func(b *planBuilder) error { b.opts = cloneOptions(opts); return nil }
}

func WithExtensions(exts ...string) Option {
	return func(b *planBuilder) error {
		for _, ext := range exts {
			ext = strings.TrimPrefix(strings.ToLower(ext), ".")
			if ext != "" {
				b.opts.Extensions = append(b.opts.Extensions, ext)
			}
		}
		return nil
	}
}

func WithExcludeGlobs(globs ...string) Option {
	return func(b *planBuilder) error {
		for _, glob := range globs {
			if glob == "" {
				continue
			}
			if !strings.HasPrefix(glob, "!") {
				glob = "!" + glob
			}
			b.opts.OverrideRules = append(b.opts.OverrideRules, glob)
		}
		return nil
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(b *planBuilder) error { b.log = logger; return nil }
}

func WithFailFast() Option {
	return func(b *planBuilder) error {
		b.failFast = true
		b.opts.Walk.ErrorPolicy = ErrorAbort
		return nil
	}
}

func WithMaxFileBytes(n uint64) Option {
	return func(b *planBuilder) error {
		b.maxSet = true
		b.opts.MaxFileBytes = n
		return nil
	}
}

func WithHashContents(on bool) Option {
	return func(b *planBuilder) error {
		b.hashSet = true
		b.opts.HashFileContents = on
		return nil
	}
}

func WithDetectBinary(on bool) Option {
	return func(b *planBuilder) error {
		b.binarySet = true
		b.opts.DetectBinaryFiles = on
		return nil
	}
}

func WithFilenameRegex(patterns ...string) Option {
	return func(b *planBuilder) error {
		exprs, err := compileRegexes(patterns)
		if err != nil {
			return err
		}
		b.opts.Filters.IncludeNameRegex = append(b.opts.Filters.IncludeNameRegex, exprs...)
		return nil
	}
}

func WithProgress(fn func(Progress)) Option {
	return func(b *planBuilder) error { b.progress = fn; return nil }
}

func WithRequireCache() Option {
	return func(b *planBuilder) error { b.requireCache = true; return nil }
}

func WithAdmitTimeout(d time.Duration) Option {
	return func(b *planBuilder) error {
		b.opts.AdmitTimeout = d
		return nil
	}
}

func WithReadLimit(lim ByteLimit) Option {
	return func(b *planBuilder) error {
		switch lim.Mode {
		case dx.LimitZero:
			b.opts.zeroByteLimit, b.opts.MaxFileBytes, b.maxSet = true, 0, true
		case dx.LimitUnlimited:
			b.opts.zeroByteLimit, b.opts.MaxFileBytes, b.maxSet = false, 0, true
		case dx.LimitValue:
			b.opts.zeroByteLimit, b.opts.MaxFileBytes, b.maxSet = false, lim.N, true
		}
		return nil
	}
}

func (p *Plan) Describe() string {
	o := p.opts
	return dx.Describe(o.IgnoreFiles, o.SkipHidden, o.StandardSkips, o.HashFileContents, o.DetectBinaryFiles, p.failFast, o.MaxFileBytes)
}

func (p *Plan) scanner(root string) (*Scanner, error) {
	if p == nil {
		return nil, &Error{Code: CodeInvalid, Op: "Plan", Err: errEmptyRoot}
	}
	return NewScanner(root, WithOptions(p.opts))
}

func (p *Plan) logFinish(ctx context.Context, event string, attrs ...slog.Attr) {
	if p == nil || p.log == nil || !p.log.Enabled(ctx, slog.LevelInfo) {
		return
	}
	p.log.LogAttrs(ctx, slog.LevelInfo, event, attrs...)
}
