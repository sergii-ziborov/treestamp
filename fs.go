// Package treestamp is a native Go port of Weavatrix Scan.
//
// The repository is the personal public project of Sergii Ziborov
// (github.com/sergii-ziborov/treestamp). It is not published from the
// Weavatrix or EdgeHawk organizations.
//
// Walk, select, hash, cache, and incremental surfaces are implemented. This
// is still not a claim that every rust differential and official bench is
// closed.
package treestamp

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/sergii-ziborov/treestamp/internal/dirread"
	"github.com/sergii-ziborov/treestamp/internal/scan"
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

var errPostChildrenUnsupported = errors.New("PostChildrenCallback requires NumWorkers=0 and FollowSymbolicLinks=false")

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
	NumWorkers           int
	ToSlash              bool
}

func WalkDirs(root string, opts DirWalkOptions) error {
	fn := opts.Callback
	if opts.ErrorCallback != nil {
		inner := fn
		fn = func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return opts.ErrorCallback(path, err)
			}
			if inner == nil {
				return nil
			}
			return inner(path, d, err)
		}
	}
	if opts.PostChildrenCallback != nil {
		if opts.NumWorkers != 0 || opts.FollowSymbolicLinks {
			return errPostChildrenUnsupported
		}
		return walk.WalkCallbackHooks(root, fn, opts.PostChildrenCallback, opts.ToSlash, opts.ContentsFirst)
	}
	cfg := Config{
		Follow: opts.FollowSymbolicLinks, ToSlash: opts.ToSlash,
		ContentsFirst: opts.ContentsFirst, DirsFirst: opts.DirsFirst,
	}
	if opts.Unsorted {
		cfg.NumWorkers = -1
	} else if opts.NumWorkers != 0 {
		cfg.NumWorkers = opts.NumWorkers
	}
	return WalkWithConfig(root, cfg, fn)
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
	return streamWalkerPaths(w, root, opts)
}

func streamWalkerPaths(w *FileWalker, root string, opts Options) error {
	return scan.StreamPaths(walkerContext(w), root, toScanOptions(opts), func(rel string) error {
		return pushWalkerPath(w, root, rel)
	})
}

func emitScannedFiles(w *FileWalker, root string, scanner *Scanner) error {
	report, err := scanner.Scan(walkerContext(w))
	if err != nil {
		return err
	}
	var paths []string
	for _, file := range report.Files {
		paths = append(paths, file.Relative)
	}
	return pushWalkerPaths(w, root, paths)
}

func walkerContext(w *FileWalker) context.Context {
	if w != nil && w.ctx != nil {
		return w.ctx
	}
	return context.Background()
}

func pushWalkerPaths(w *FileWalker, root string, paths []string) error {
	for _, rel := range paths {
		if err := pushWalkerPath(w, root, rel); err != nil {
			return err
		}
	}
	return nil
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
