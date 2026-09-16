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
}

func discover(ctx context.Context, root string, opts Options, needMeta bool) (*discovery, error) {
	if opts.Started.IsZero() {
		opts.Started = time.Now()
	}
	walker, matcher, snapshots, out, err := openDiscovery(root, opts, needMeta)
	if err != nil {
		return nil, err
	}
	defer walker.Close()
	var selected, totalBytes uint64
	mergeIgnoreSources(&out.sources, matcher.Sources())
	for {
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
		considerEntry(considerArgs{walker: walker, matcher: matcher, snapshots: &snapshots, out: out, opts: opts, entry: entry, needMeta: needMeta}, &selected, &totalBytes)
	}
	mergeIgnoreSources(&out.sources, matcher.Sources())
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
	out := &discovery{root: walker.Root(), complete: true, portable: portablePolicy(opts)}
	return walker, matcher, []*ignore.Engine{matcher.Engine().Clone()}, out, nil
}

func selectionConfig(opts Options, walkOpts walk.WalkOptions, needMeta bool) selection.Config {
	return selection.Config{
		IgnoreFiles: opts.IgnoreFiles, IgnoreCase: opts.IgnoreCase, IgnorePolicy: opts.IgnorePolicy,
		OverrideRules: opts.OverrideRules, Extensions: opts.Extensions, FileTypes: opts.FileTypes,
		SkipHidden: opts.SkipHidden, StandardSkips: opts.StandardSkips, GitModules: opts.GitModules,
		Filters: opts.Filters,
		MaxFileBytes:  opts.MaxFileBytes,
		ApplyMaxBytes: needMeta, MinDepth: walkOpts.MinDepth, MaxDepth: walkOpts.MaxDepth,
	}
}

type considerArgs struct {
	walker    *walk.Walker
	matcher   *selection.Matcher
	snapshots *[]*ignore.Engine
	out       *discovery
	opts      Options
	entry     *walk.WalkEntry
	needMeta  bool
}

func considerEntry(a considerArgs, selected, totalBytes *uint64) {
	rel := pathx.Slash(a.entry.RelativePath())
	restoreMatcher(a.matcher, *a.snapshots, a.entry.Depth())
	dec := a.matcher.Decide(a.entry)
	if a.entry.IsDir() && dec.ShouldDescend() && a.entry.Depth() > 0 {
		for _, w := range a.matcher.EnterDir(a.entry.Path(), rel) {
			a.out.warnings = append(a.out.warnings, Warning{Relative: rel, Message: w})
			a.out.complete = false
		}
		mergeIgnoreSources(&a.out.sources, a.matcher.Sources())
		for len(*a.snapshots) <= a.entry.Depth() {
			*a.snapshots = append(*a.snapshots, a.matcher.Engine().Clone())
		}
		(*a.snapshots)[a.entry.Depth()] = a.matcher.Engine().Clone()
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
	a.out.paths = append(a.out.paths, rel)
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
	if a.opts.MaxFileBytes > 0 && size > a.opts.MaxFileBytes {
		if a.opts.RecordSkipped {
			a.out.skipped = append(a.out.skipped, Skipped{Relative: rel, Kind: selection.SkipOversized})
		}
		return
	}
	a.out.candidates = append(a.out.candidates, candidate{abs: a.entry.Path(), rel: rel, size: size, version: version})
	*selected++
	*totalBytes += size
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

func restoreMatcher(matcher *selection.Matcher, snaps []*ignore.Engine, depth int) {
	idx := 0
	if depth > 0 {
		idx = depth - 1
	}
	if idx >= 0 && idx < len(snaps) && snaps[idx] != nil {
		*matcher.Engine() = *snaps[idx].Clone()
	}
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
