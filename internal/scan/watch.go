package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/sergii-ziborov/treestamp/internal/fileread"
	"github.com/sergii-ziborov/treestamp/internal/ignore"
	pathx "github.com/sergii-ziborov/treestamp/internal/path"
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
		if pathx.IsSameOrDescendant(relative, prefix) {
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
	plan = sanitizePlan(plan)
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
	set, err := collectChanges(scope, true)
	if err != nil {
		return nil, err
	}
	if set.structural != nil {
		return set.structural, nil
	}
	if ignoreChanged(previous, matcher.Sources(), plan) {
		return rescanUpdate(ctx, root, opts, previous, WatchFullIgnore)
	}
	return finishWatch(scope, kept, set)
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

type changeSet struct {
	candidates []candidate
	skipped    []Skipped
	structural *WatchUpdate
}

func collectChanges(s watchScope, dirMeansStructural bool) (changeSet, error) {
	var set changeSet
	for _, rel := range s.plan.Changed {
		if err := s.ctx.Err(); err != nil {
			return set, err
		}
		item, update, err := changeOne(s, rel, dirMeansStructural)
		if err != nil || update != nil {
			set.structural = update
			return set, err
		}
		if item.skip != nil {
			set.skipped = append(set.skipped, *item.skip)
			continue
		}
		if item.ok {
			set.candidates = append(set.candidates, item.cand)
		}
	}
	return set, nil
}

type changeItem struct {
	cand candidate
	skip *Skipped
	ok   bool
}

func changeOne(s watchScope, rel string, dirMeansStructural bool) (changeItem, *WatchUpdate, error) {
	if _, ok := pathx.SafeRelative(rel); !ok {
		return changeItem{skip: &Skipped{Relative: rel, Kind: selection.SkipPathEscape}}, nil, nil
	}
	path := filepath.Join(s.abs, filepath.FromSlash(rel))
	if !pathx.UnderRoot(s.abs, path) {
		return changeItem{skip: &Skipped{Relative: rel, Kind: selection.SkipPathEscape}}, nil, nil
	}
	info, statErr := os.Lstat(path)
	if statErr != nil {
		return changeItem{skip: &Skipped{Relative: rel, Kind: selection.SkipIOError, Detail: statErr.Error()}}, nil, nil
	}
	if _, err := fileread.ConfineAt(s.abs, path, s.opts.Walk.FollowLinks); err != nil {
		return changeItem{skip: &Skipped{Relative: rel, Kind: confineSkipKind(err), Detail: err.Error()}}, nil, nil
	}
	if info.IsDir() {
		if !dirMeansStructural {
			return changeItem{}, nil, nil
		}
		update, err := rescanUpdate(s.ctx, s.root, s.opts, s.previous, WatchFullStructural)
		return changeItem{}, update, err
	}
	return selectChanged(s, rel, path, info)
}

func selectChanged(s watchScope, rel, path string, info os.FileInfo) (changeItem, *WatchUpdate, error) {
	_ = s.matcher.LoadPathScope(rel)
	size := uint64(info.Size())
	dec := s.matcher.DecideWithAncestors(selection.PathQuery{
		Rel: rel, Name: filepath.Base(rel), IsFile: info.Mode().IsRegular(),
		IsSymlink: info.Mode()&os.ModeSymlink != 0, Size: &size,
	})
	if !dec.IsSelected() {
		if s.opts.RecordSkipped && dec.Skip != selection.SkipNone {
			return changeItem{skip: &Skipped{Relative: rel, Kind: dec.Skip}}, nil, nil
		}
		return changeItem{}, nil, nil
	}
	return changeItem{ok: true, cand: candidate{
		abs: path, rel: rel, size: size, version: fileread.FromInfo(path, info),
	}}, nil, nil
}

func finishWatch(s watchScope, kept []ScannedFile, set changeSet) (*WatchUpdate, error) {
	s.opts.Root = s.abs
	inspected, extraSkip, stats, err := inspect(s.ctx, set.candidates, s.opts)
	if err != nil {
		return nil, err
	}
	prefixes := append(append([]string(nil), s.plan.Changed...), s.plan.Removed...)
	skips := append(set.skipped, extraSkip...)
	report := &Report{
		Root: s.abs, Files: append(kept, inspected...), Complete: watchComplete(s.ctx, skips),
		Portable: portablePolicy(s.opts), Cache: stats,
		IgnoreSources: mergeWatchSources(s.previous, s.matcher.Sources(), prefixes),
	}
	if s.previous != nil {
		report.Skipped = append(keepUncoveredSkips(s.previous.Skipped, prefixes), skips...)
		report.Warnings = keepUncoveredWarnings(s.previous.Warnings, prefixes)
	} else {
		report.Skipped = skips
	}
	finalize(report, s.opts)
	return &WatchUpdate{Report: report, Reason: WatchIncremental}, nil
}

func sanitizePlan(plan WatchPlan) WatchPlan {
	plan.Changed = sanitizeRels(plan.Changed, &plan.RejectedEvents)
	plan.Removed = sanitizeRels(plan.Removed, &plan.RejectedEvents)
	return plan
}

func confineSkipKind(err error) selection.SkipKind {
	if errors.Is(err, fileread.ErrEscape) || errors.Is(err, fileread.ErrSymlink) {
		return selection.SkipPathEscape
	}
	return selection.SkipIOError
}

func rejectedRels(in []string) []string {
	var out []string
	for _, rel := range in {
		if _, ok := pathx.SafeRelative(rel); !ok {
			out = append(out, rel)
		}
	}
	return out
}

func sanitizeRels(in []string, rejected *uint64) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, rel := range in {
		clean, ok := pathx.SafeRelative(rel)
		if !ok {
			*rejected++
			continue
		}
		if _, dup := seen[clean]; dup {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func keepUncoveredSkips(previous []Skipped, prefixes []string) []Skipped {
	var keep []Skipped
	for _, item := range previous {
		if !pathx.PathCoveredByPrefixes(item.Relative, prefixes) {
			keep = append(keep, item)
		}
	}
	return keep
}

func keepUncoveredWarnings(previous []Warning, prefixes []string) []Warning {
	var keep []Warning
	for _, item := range previous {
		if !pathx.PathCoveredByPrefixes(item.Relative, prefixes) {
			keep = append(keep, item)
		}
	}
	return keep
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

func ignoreChanged(previous *Report, current []ignore.Source, plan WatchPlan) bool {
	if previous == nil {
		return false
	}
	prev := map[string]string{}
	for _, source := range previous.IgnoreSources {
		prev[source.Location] = source.ContentHash
	}
	cur := map[string]string{}
	for _, source := range current {
		cur[source.Location] = source.ContentHash
		if prev[source.Location] != source.ContentHash {
			return true
		}
	}
	for loc, hash := range prev {
		if sourceAffectsAny(loc, plan.Invalidated()) && cur[loc] != hash {
			return true
		}
	}
	return false
}

func watchComplete(ctx context.Context, skips []Skipped) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	for _, item := range skips {
		switch item.Kind {
		case selection.SkipIOError, selection.SkipPathEscape, selection.SkipConcurrentModification:
			return false
		}
	}
	return true
}

func mergeWatchSources(previous *Report, current []ignore.Source, prefixes []string) []IgnoreSource {
	next := map[string]IgnoreSource{}
	if previous != nil {
		for _, source := range previous.IgnoreSources {
			if !sourceAffectsAny(source.Location, prefixes) {
				next[source.Location] = source
			}
		}
	}
	for _, source := range current {
		next[source.Location] = IgnoreSource{Kind: source.Kind, Location: source.Location, ContentHash: source.ContentHash}
	}
	out := make([]IgnoreSource, 0, len(next))
	for _, source := range next {
		out = append(out, source)
	}
	return out
}

func sourceAffectsAny(location string, rels []string) bool {
	for _, rel := range rels {
		if sourceAffects(location, rel) {
			return true
		}
	}
	return false
}

func sourceAffects(location, rel string) bool {
	dir := sourceDir(location)
	if dir == "" {
		return true
	}
	rel = pathx.Slash(rel)
	return rel == dir || strings.HasPrefix(rel, dir+"/")
}

func sourceDir(location string) string {
	if location == "" || strings.HasPrefix(location, "<") {
		return ""
	}
	slash := strings.ReplaceAll(location, "\\", "/")
	i := strings.LastIndexByte(slash, '/')
	if i < 0 {
		return ""
	}
	return slash[:i]
}

func toIgnoreSources(in []ignore.Source) []IgnoreSource {
	out := make([]IgnoreSource, 0, len(in))
	for _, source := range in {
		out = append(out, IgnoreSource{Kind: source.Kind, Location: source.Location, ContentHash: source.ContentHash})
	}
	return out
}
