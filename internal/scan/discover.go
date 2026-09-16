package scan

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sergii-ziborov/treestamp/internal/ignore"
	pathx "github.com/sergii-ziborov/treestamp/internal/path"
	"github.com/sergii-ziborov/treestamp/internal/selection"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

type candidate struct {
	abs     string
	rel     string
	size    uint64
	version walk.FileVersion
}

type discovery struct {
	root       string
	paths      []string
	candidates []candidate
	skipped    []Skipped
	warnings   []Warning
	sources    []IgnoreSource
	term       Termination
	complete   bool
	portable   bool
	emit       func(string) error
	emitErr    error
}

func discover(ctx context.Context, root string, opts Options, needMeta bool) (*discovery, error) {
	if opts.Started.IsZero() {
		opts.Started = time.Now()
	}
	if !needMeta && listable(opts) {
		return listDiscover(ctx, root, opts)
	}
	walker, matcher, snapshots, out, err := openDiscovery(root, opts, needMeta)
	if err != nil {
		return nil, err
	}
	defer walker.Close()
	var selected, totalBytes uint64
	needSources := needMeta || opts.RecordSkipped
	if needSources {
		mergeIgnoreSources(&out.sources, matcher.Sources())
	}
	lastSnap := -1
	for {
		if out.emitErr != nil {
			out.complete = false
			return out, nil
		}
		if err := ctx.Err(); err != nil {
			out.term = TermCancelled
			out.complete = false
			return out, nil
		}
		if stopped, term := limitsHit(opts, selected, totalBytes); stopped {
			out.term = term
			out.complete = false
			break
		}
		entry, err := walker.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if opts.Walk.ErrorPolicy == walk.ErrorAbort {
				return nil, err
			}
			if opts.RecordSkipped {
				out.skipped = append(out.skipped, Skipped{Kind: selection.SkipIOError, Detail: err.Error()})
			}
			out.complete = false
			continue
		}
		considerEntry(considerArgs{
			walker: walker, matcher: matcher, snapshots: &snapshots, out: out, opts: opts,
			entry: entry, needMeta: needMeta, needSources: needSources, lastSnap: &lastSnap,
		}, &selected, &totalBytes)
	}
	if needSources {
		mergeIgnoreSources(&out.sources, matcher.Sources())
	}
	if opts.IgnorePolicy.GitGlobal || opts.IgnorePolicy.GitExclude {
		out.portable = false
	}
	return out, nil
}

func openDiscovery(root string, opts Options, needMeta bool) (*walk.Walker, *selection.Matcher, []*ignore.Engine, *discovery, error) {
	if root == "" {
		return nil, nil, nil, nil, os.ErrInvalid
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, nil, nil, &os.PathError{Op: "scan", Path: abs, Err: errNotDir}
	}
	walkOpts := opts.Walk
	if needMeta {
		walkOpts.CollectMetadata = true
	}
	walker, err := walk.NewWithOptions(abs, walkOpts)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	matcher, err := selection.NewMatcher(walker.Root(), selectionConfig(opts, walkOpts, needMeta))
	if err != nil {
		_ = walker.Close()
		return nil, nil, nil, nil, err
	}
	out := &discovery{root: walker.Root(), complete: true, portable: portablePolicy(opts), emit: opts.emitPath}
	return walker, matcher, []*ignore.Engine{matcher.Engine().Clone()}, out, nil
}

func selectionConfig(opts Options, walkOpts walk.WalkOptions, needMeta bool) selection.Config {
	return selection.Config{
		IgnoreFiles: opts.IgnoreFiles, IgnoreCase: opts.IgnoreCase, IgnorePolicy: opts.IgnorePolicy,
		OverrideRules: opts.OverrideRules, Extensions: opts.Extensions, FileTypes: opts.FileTypes,
		SkipHidden: opts.SkipHidden, StandardSkips: opts.StandardSkips, GitModules: opts.GitModules,
		Filters:       opts.Filters,
		MaxFileBytes:  opts.MaxFileBytes,
		ApplyMaxBytes: needMeta, MinDepth: walkOpts.MinDepth, MaxDepth: walkOpts.MaxDepth,
	}
}

type considerArgs struct {
	walker      *walk.Walker
	matcher     *selection.Matcher
	snapshots   *[]*ignore.Engine
	out         *discovery
	opts        Options
	entry       *walk.WalkEntry
	needMeta    bool
	needSources bool
	lastSnap    *int
}

func considerEntry(a considerArgs, selected, totalBytes *uint64) {
	rel := pathx.Slash(a.entry.RelativePath())
	restoreMatcher(a.matcher, *a.snapshots, a.entry.Depth(), a.lastSnap)
	dec := a.matcher.Decide(a.entry)
	if a.entry.IsDir() && dec.ShouldDescend() && !a.opts.RecordSkipped && a.matcher.MayContain(rel) == selection.ContainNo {
		a.walker.SkipCurrentDir()
		return
	}
	if a.entry.IsDir() && dec.ShouldDescend() && a.entry.Depth() > 0 {
		detachEngine(a.matcher)
		for _, w := range a.matcher.EnterDir(a.entry.Path(), rel) {
			a.out.warnings = append(a.out.warnings, Warning{Relative: rel, Message: w})
			a.out.complete = false
		}
		if a.needSources {
			mergeIgnoreSources(&a.out.sources, a.matcher.Sources())
		}
		for len(*a.snapshots) <= a.entry.Depth() {
			*a.snapshots = append(*a.snapshots, nil)
		}
		(*a.snapshots)[a.entry.Depth()] = a.matcher.Engine().Clone()
		*a.lastSnap = a.entry.Depth()
		return
	}
	if !dec.ShouldDescend() && a.entry.IsDir() {
		a.walker.SkipCurrentDir()
	}
	if dec.IsSelected() {
		recordSelected(a, rel, selected, totalBytes)
		return
	}
	if a.opts.RecordSkipped && dec.Disposition == selection.Skipped {
		a.out.skipped = append(a.out.skipped, Skipped{Relative: rel, Kind: dec.Skip})
	}
}

func recordSelected(a considerArgs, rel string, selected, totalBytes *uint64) {
	if !emitSelected(a.out, rel) {
		return
	}
	if !a.needMeta {
		*selected++
		return
	}
	var size uint64
	var version walk.FileVersion
	if a.entry.Bytes() != nil {
		size = *a.entry.Bytes()
	} else if info, err := os.Stat(a.entry.Path()); err == nil {
		size = uint64(info.Size())
	}
	if a.entry.Version() != nil {
		version = *a.entry.Version()
	}
	if oversized(a.opts, size) {
		if a.opts.RecordSkipped {
			a.out.skipped = append(a.out.skipped, Skipped{Relative: rel, Kind: selection.SkipOversized})
		}
		return
	}
	a.out.candidates = append(a.out.candidates, candidate{abs: a.entry.Path(), rel: rel, size: size, version: version})
	*selected++
	*totalBytes += size
}

func oversized(opts Options, size uint64) bool {
	if opts.MaxFileBytesZero {
		return size > 0
	}
	return opts.MaxFileBytes > 0 && size > opts.MaxFileBytes
}

func emitSelected(out *discovery, rel string) bool {
	if out.emit != nil {
		if err := out.emit(rel); err != nil {
			out.emitErr = err
			out.complete = false
			return false
		}
		return true
	}
	out.paths = append(out.paths, rel)
	return true
}

func mergeIgnoreSources(dst *[]IgnoreSource, src []ignore.Source) {
	for _, source := range src {
		item := IgnoreSource{Kind: source.Kind, Location: source.Location, ContentHash: source.ContentHash}
		dup := false
		for _, have := range *dst {
			if have.Kind == item.Kind && have.Location == item.Location && have.ContentHash == item.ContentHash {
				dup = true
				break
			}
		}
		if !dup {
			*dst = append(*dst, item)
		}
	}
}

func restoreMatcher(matcher *selection.Matcher, snaps []*ignore.Engine, depth int, last *int) {
	idx := 0
	if depth > 0 {
		idx = depth - 1
	}
	if last != nil && *last == idx {
		return
	}
	if idx >= 0 && idx < len(snaps) && snaps[idx] != nil {
		*matcher.Engine() = *snaps[idx]
		if last != nil {
			*last = idx
		}
	}
}

func detachEngine(matcher *selection.Matcher) {
	eng := matcher.Engine()
	*eng = *eng.Clone()
}

func listable(opts Options) bool {
	w := opts.Walk
	return !w.FollowLinks && !w.SameFileSystem && w.MinDepth == 0 && w.MaxDepth == nil
}

type listWalker struct {
	ctx     context.Context
	matcher *selection.Matcher
	out     *discovery
	opts    Options
	picked  uint64
}

func listDiscover(ctx context.Context, root string, opts Options) (*discovery, error) {
	if root == "" {
		return nil, os.ErrInvalid
	}
	matcher, err := selection.NewDeferredMatcher(root, selectionConfig(opts, opts.Walk, false))
	if err != nil {
		return nil, err
	}
	dents, err := os.ReadDir(matcher.Root())
	if err != nil {
		return nil, err
	}
	out := &discovery{root: matcher.Root(), complete: true, portable: portablePolicy(opts), emit: opts.emitPath}
	w := &listWalker{ctx: ctx, matcher: matcher, out: out, opts: opts}
	w.noteListed(matcher.Root(), "", dents)
	w.walk(matcher.Root(), "", 0, dents)
	return out, nil
}

func (w *listWalker) walk(abs, rel string, depth int, dents []os.DirEntry) {
	if w.out.emitErr != nil {
		return
	}
	if w.ctx.Err() != nil {
		w.out.term = TermCancelled
		w.out.complete = false
		return
	}
	if stopped, term := limitsHit(w.opts, w.picked, 0); stopped {
		w.out.term = term
		w.out.complete = false
		return
	}
	w.visitFiles(rel, dents)
	if w.out.term != TermNone {
		return
	}
	w.visitDirs(abs, rel, depth, dents)
}

func (w *listWalker) visitFiles(rel string, dents []os.DirEntry) {
	for i := range dents {
		isDir, isFile, symlink := dentKind(dents[i])
		if isDir {
			continue
		}
		name := dents[i].Name()
		fileRel := name
		if rel != "" {
			fileRel = rel + "/" + name
		}
		if !w.keepFile(fileRel, name, isFile, symlink) {
			continue
		}
		if !emitSelected(w.out, fileRel) {
			return
		}
		w.picked++
	}
}

func (w *listWalker) keepFile(rel, name string, isFile, symlink bool) bool {
	if keep, fast := w.matcher.KeepListedFile(name, isFile, symlink); fast {
		return keep
	}
	return w.matcher.DecidePath(selection.PathQuery{
		Rel: rel, Name: name, IsFile: isFile, IsSymlink: symlink,
	}).IsSelected()
}

func (w *listWalker) visitDirs(abs, rel string, depth int, dents []os.DirEntry) {
	for i := range dents {
		isDir, _, _ := dentKind(dents[i])
		if !isDir {
			continue
		}
		name := dents[i].Name()
		childRel := name
		if rel != "" {
			childRel = rel + "/" + name
		}
		if !w.keepDir(childRel, name) {
			continue
		}
		w.enter(joinChild(abs, name), childRel, depth+1)
	}
}

func (w *listWalker) keepDir(rel, name string) bool {
	if w.matcher.MayContain(rel) == selection.ContainNo {
		return false
	}
	if keep, fast := w.matcher.KeepListedDir(rel, name); fast {
		return keep
	}
	return w.matcher.DecidePath(selection.PathQuery{Rel: rel, Name: name, IsDir: true}).ShouldDescend()
}

func (w *listWalker) enter(abs, rel string, depth int) {
	dents, err := os.ReadDir(abs)
	if err != nil {
		w.out.complete = false
		return
	}
	for _, msg := range w.matcher.WithListed(abs, rel, dents, func() {
		w.walk(abs, rel, depth, dents)
	}) {
		w.out.warnings = append(w.out.warnings, Warning{Relative: rel, Message: msg})
		w.out.complete = false
	}
}

func (w *listWalker) noteListed(abs, rel string, dents []os.DirEntry) {
	for _, msg := range w.matcher.EnterDirListed(abs, rel, dents) {
		w.out.warnings = append(w.out.warnings, Warning{Relative: rel, Message: msg})
		w.out.complete = false
	}
}

func dentKind(dent os.DirEntry) (isDir, isFile, symlink bool) {
	mode := dent.Type()
	if mode == 0 {
		if info, err := dent.Info(); err == nil {
			mode = info.Mode()
		}
	}
	symlink = mode&os.ModeSymlink != 0
	return !symlink && mode.IsDir(), !symlink && mode.IsRegular(), symlink
}

func joinChild(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + string(os.PathSeparator) + name
}

func limitsHit(opts Options, selected, total uint64) (bool, Termination) {
	if opts.Cancel != nil && opts.Cancel.Cancelled() {
		return true, TermCancelled
	}
	if opts.Limits.Timeout > 0 && !opts.Started.IsZero() && time.Since(opts.Started) >= opts.Limits.Timeout {
		return true, TermTimeout
	}
	if opts.Limits.MaxEntries != nil && selected >= *opts.Limits.MaxEntries {
		return true, TermMaxEntries
	}
	if opts.Limits.MaxTotalBytes != nil && total >= *opts.Limits.MaxTotalBytes {
		return true, TermMaxTotalBytes
	}
	return false, TermNone
}

func portablePolicy(opts Options) bool {
	p := opts.IgnorePolicy
	if !p.Specified() {
		p = ignore.RepositoryPolicy()
	}
	return !p.GitGlobal && !p.GitExclude && !p.ParentRules
}

var errNotDir = errText("scan root must be a directory")

type errText string

func (e errText) Error() string { return string(e) }
