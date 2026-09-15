package scan

import (
	"context"
	"os"
	"path/filepath"

	"github.com/sergii-ziborov/treestamp/internal/ignore"
	"github.com/sergii-ziborov/treestamp/internal/selection"
)

type WatchPlan struct {
	Changed        []string
	Removed        []string
	FullRescan     bool
	RejectedEvents uint64
}

func (p WatchPlan) Invalidated() []string {
	return append(append([]string(nil), p.Changed...), p.Removed...)
}

func (p WatchPlan) Invalidates(relative string) bool {
	for _, prefix := range p.Invalidated() {
		if relative == prefix || (len(relative) > len(prefix) && relative[:len(prefix)] == prefix && relative[len(prefix)] == '/') {
			return true
		}
	}
	return false
}

type WatchReason int

const (
	WatchIncremental WatchReason = iota
	WatchFullPolicy
	WatchFullIgnore
	WatchFullStructural
	WatchFullIncomplete
)

func (r WatchReason) String() string {
	switch r {
	case WatchIncremental:
		return "Incremental"
	case WatchFullPolicy:
		return "FullRescan:PolicyChanged"
	case WatchFullIgnore:
		return "FullRescan:IgnoreInputChanged"
	case WatchFullStructural:
		return "FullRescan:StructuralChange"
	case WatchFullIncomplete:
		return "FullRescan:IncompletePreviousState"
	default:
		return ""
	}
}

type WatchUpdate struct {
	Report *Report
	Reason WatchReason
}

func Watch(ctx context.Context, root string, opts Options, previous *Report, plan WatchPlan) (*WatchUpdate, error) {
	if reason, ok := earlyRescan(previous, plan, opts); ok {
		return rescanUpdate(ctx, root, opts, previous, reason)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if previous != nil && abs != previous.Root {
		return rescanFull(ctx, root, opts, WatchFullStructural)
	}
	kept := keepUnchanged(previous, plan)
	matcher, err := selection.NewMatcher(abs, selectionConfig(opts, opts.Walk, true))
	if err != nil {
		return nil, err
	}
	scope := watchScope{ctx: ctx, root: root, abs: abs, opts: opts, previous: previous, matcher: matcher, plan: plan}
	candidates, structural, err := changedCandidates(scope)
	if err != nil {
		return nil, err
	}
	if structural != nil {
		return structural, nil
	}
	if ignoreChanged(previous, matcher.Sources()) {
		return rescanUpdate(ctx, root, opts, previous, WatchFullIgnore)
	}
	return finishWatch(scope, kept, candidates)
}

func rescanUpdate(ctx context.Context, root string, opts Options, previous *Report, reason WatchReason) (*WatchUpdate, error) {
	report, err := Incremental(ctx, root, opts, previous)
	if err != nil {
		return nil, err
	}
	return &WatchUpdate{Report: report, Reason: reason}, nil
}

func rescanFull(ctx context.Context, root string, opts Options, reason WatchReason) (*WatchUpdate, error) {
	report, err := Full(ctx, root, opts)
	if err != nil {
		return nil, err
	}
	return &WatchUpdate{Report: report, Reason: reason}, nil
}

func keepUnchanged(previous *Report, plan WatchPlan) []ScannedFile {
	var kept []ScannedFile
	if previous == nil {
		return kept
	}
	for _, file := range previous.Files {
		if !plan.Invalidates(file.Relative) {
			kept = append(kept, file)
		}
	}
	return kept
}

type watchScope struct {
	ctx      context.Context
	root     string
	abs      string
	opts     Options
	previous *Report
	matcher  *selection.Matcher
	plan     WatchPlan
}

func changedCandidates(s watchScope) ([]candidate, *WatchUpdate, error) {
	var candidates []candidate
	for _, rel := range s.plan.Changed {
		path := filepath.Join(s.abs, filepath.FromSlash(rel))
		info, statErr := os.Lstat(path)
		if statErr != nil {
			continue
		}
		if info.IsDir() {
			update, err := rescanUpdate(s.ctx, s.root, s.opts, s.previous, WatchFullStructural)
			return nil, update, err
		}
		size := uint64(info.Size())
		dec := s.matcher.DecidePath(selection.PathQuery{
			Rel: rel, Name: filepath.Base(rel), IsFile: info.Mode().IsRegular(),
			IsSymlink: info.Mode()&os.ModeSymlink != 0, Size: &size,
		})
		if dec.IsSelected() {
			candidates = append(candidates, candidate{abs: path, rel: rel, size: size})
		}
	}
	return candidates, nil, nil
}

func finishWatch(s watchScope, kept []ScannedFile, candidates []candidate) (*WatchUpdate, error) {
	inspected, extraSkip, stats, err := inspect(s.ctx, candidates, s.opts)
	if err != nil {
		return nil, err
	}
	report := &Report{
		Root: s.abs, Files: append(kept, inspected...), Skipped: extraSkip, Complete: true,
		Portable: portablePolicy(s.opts), Cache: stats, IgnoreSources: toIgnoreSources(s.matcher.Sources()),
	}
	if s.previous != nil {
		report.Skipped = append(append([]Skipped(nil), s.previous.Skipped...), report.Skipped...)
		report.Warnings = append([]Warning(nil), s.previous.Warnings...)
	}
	finalize(report, s.opts)
	return &WatchUpdate{Report: report, Reason: WatchIncremental}, nil
}

func earlyRescan(previous *Report, plan WatchPlan, opts Options) (WatchReason, bool) {
	if plan.FullRescan {
		return WatchFullStructural, true
	}
	if previous == nil || !previous.Complete || previous.Termination != TermNone || opts.Limits.MaxEntries != nil {
		return WatchFullIncomplete, true
	}
	if !previous.Descriptor.Matches(opts) {
		return WatchFullPolicy, true
	}
	return 0, false
}

func ignoreChanged(previous *Report, current []ignore.Source) bool {
	if previous == nil {
		return false
	}
	if len(previous.IgnoreSources) != len(current) {
		return true
	}
	seen := make(map[string]string, len(previous.IgnoreSources))
	for _, source := range previous.IgnoreSources {
		seen[source.Location] = source.ContentHash
	}
	for _, source := range current {
		if seen[source.Location] != source.ContentHash {
			return true
		}
	}
	return false
}

func toIgnoreSources(in []ignore.Source) []IgnoreSource {
	out := make([]IgnoreSource, 0, len(in))
	for _, source := range in {
		out = append(out, IgnoreSource{Kind: source.Kind, Location: source.Location, ContentHash: source.ContentHash})
	}
	return out
}
