package treestamp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pathx "github.com/sergii-ziborov/treestamp/internal/path"
	"github.com/sergii-ziborov/treestamp/internal/report"
	"github.com/sergii-ziborov/treestamp/internal/scan"
)

type ContentVisitMode int

const (
	ContentVisitRevision ContentVisitMode = iota
	ContentVisitStreaming
)

type ContentVisitControl int

const (
	ContentVisitContinue ContentVisitControl = iota
	ContentVisitSkipFile
	ContentVisitQuit
)

type ContentFile struct {
	RootIndex                uint
	Sequence                 uint64
	Root, Absolute, Relative string
	Bytes                    uint64
}

type ContentFileStatus int

const (
	ContentFileSelected ContentFileStatus = iota
	ContentFileBinary
	ContentFileChanged
)

type ContentVisitEvent struct {
	FileStart, Chunk, FileEnd bool
	WorkerIndex               int
	File                      ContentFile
	Offset                    uint64
	Bytes                     []byte
	Status                    ContentFileStatus
	BytesRead                 uint64
	ContentHash               string
	ConsumerSkipped           bool
}

type ContentVisitor func(ContentVisitEvent) ContentVisitControl

type ContentVisitReport struct {
	Mode                                                                            ContentVisitMode
	Root                                                                            string
	Discovered, Completed, Opened, Chunks, BytesRead, BytesEmitted, ConsumerSkipped uint64
	Stopped                                                                         bool
	Skipped                                                                         []SkippedEntry
	Warnings                                                                        []ScanWarning
	IgnoreSources                                                                   []IgnoreSourceEvidence
	Revision                                                                        string
	Complete                                                                        bool
	Termination                                                                     ScanTermination
	Portable                                                                        bool
	Cache                                                                           ScanCacheStats
}

type MultiContentVisitReport struct{ Reports []ContentVisitReport }

func (r MultiContentVisitReport) Len() int      { return len(r.Reports) }
func (r MultiContentVisitReport) IsEmpty() bool { return len(r.Reports) == 0 }

type ChangedContentVisitReport struct {
	Content ContentVisitReport
	Removed []string
}
type ChangedContentVisitOutcome struct {
	Visited            *ChangedContentVisitReport
	FullRescanRequired bool
}

func (s *Scanner) VisitContent(ctx context.Context, factory func(worker int) ContentVisitor) (*ContentVisitReport, error) {
	return s.visitContent(ctx, ContentVisitRevision, factory)
}
func (s *Scanner) VisitContentStreaming(ctx context.Context, factory func(worker int) ContentVisitor) (*ContentVisitReport, error) {
	return s.visitContent(ctx, ContentVisitStreaming, factory)
}
func (s *Scanner) VisitContentManifest(ctx context.Context, factory func(worker int) ContentVisitor) (*CompactScanReport, error) {
	inner, err := scan.VisitContent(ctx, s.root, toScanOptions(s.options), scan.VisitRevision, func(worker int) scan.ContentVisitor {
		visitor := factory(worker)
		return func(ev scan.ContentVisitEvent) scan.ContentVisitControl {
			return scan.ContentVisitControl(visitor(fromContentEvent(ev)))
		}
	})
	if err != nil {
		return nil, wrap(err, "VisitContentManifest", s.root)
	}
	if inner.Manifest == nil {
		return fromCompact(&scan.CompactReport{
			Root: inner.Root, Revision: inner.Revision, Complete: inner.Complete && !inner.Stopped,
			Termination: inner.Termination, Portable: inner.Portable, Cache: inner.Cache,
		}), nil
	}
	return fromCompact(inner.Manifest), nil
}
func (s *Scanner) VisitChangedContent(ctx context.Context, plan WatchPlan, factory func(worker int) ContentVisitor) (ChangedContentVisitOutcome, error) {
	if plan.FullRescan {
		return ChangedContentVisitOutcome{FullRescanRequired: true}, nil
	}
	inner, err := scan.VisitChanged(ctx, s.root, toScanOptions(s.options), toInternalPlan(plan), wrapContentVisitor(factory))
	if err != nil {
		return ChangedContentVisitOutcome{}, wrap(err, "VisitChangedContent", s.root)
	}
	return ChangedContentVisitOutcome{Visited: &ChangedContentVisitReport{Content: *fromContentReport(inner), Removed: plan.Removed}}, nil
}

func (s *Scanner) visitContent(ctx context.Context, mode ContentVisitMode, factory func(worker int) ContentVisitor) (*ContentVisitReport, error) {
	inner, err := scan.VisitContent(ctx, s.root, toScanOptions(s.options), scan.ContentVisitMode(mode), wrapContentVisitor(factory))
	if err != nil {
		return nil, wrap(err, "VisitContent", s.root)
	}
	return fromContentReport(inner), nil
}

func wrapContentVisitor(factory func(worker int) ContentVisitor) func(int) scan.ContentVisitor {
	return func(worker int) scan.ContentVisitor {
		visitor := factory(worker)
		return func(ev scan.ContentVisitEvent) scan.ContentVisitControl {
			return scan.ContentVisitControl(visitor(fromContentEvent(ev)))
		}
	}
}

func fromContentEvent(ev scan.ContentVisitEvent) ContentVisitEvent {
	return ContentVisitEvent{
		FileStart: ev.Kind == scan.ContentFileStart, Chunk: ev.Kind == scan.ContentChunk, FileEnd: ev.Kind == scan.ContentFileEnd,
		WorkerIndex: ev.WorkerIndex,
		File:        ContentFile{RootIndex: ev.File.RootIndex, Sequence: ev.File.Sequence, Root: ev.File.Root, Absolute: ev.File.Absolute, Relative: ev.File.Relative, Bytes: ev.File.Bytes},
		Offset:      ev.Offset, Bytes: ev.Bytes, Status: ContentFileStatus(ev.Status),
		BytesRead: ev.BytesRead, ContentHash: ev.ContentHash, ConsumerSkipped: ev.ConsumerSkipped,
	}
}

func fromContentReport(in *scan.ContentVisitReport) *ContentVisitReport {
	return &ContentVisitReport{
		Mode: ContentVisitMode(in.Mode), Root: in.Root, Discovered: in.Discovered, Completed: in.Completed,
		Opened: in.Opened, Chunks: in.Chunks, BytesRead: in.BytesRead, BytesEmitted: in.BytesEmitted,
		ConsumerSkipped: in.ConsumerSkipped, Stopped: in.Stopped, Revision: in.Revision,
		Complete: in.Complete, Termination: ScanTermination(in.Termination), Portable: in.Portable,
		Cache: ScanCacheStats(in.Cache), Skipped: copySkipped(in.Skipped), Warnings: copyWarnings(in.Warnings),
		IgnoreSources: copySources(in.IgnoreSources),
	}
}

type ScanSinkControl int

const (
	ScanSinkContinue ScanSinkControl = iota
	ScanSinkStop
)

type ScanSink interface {
	OnFile(*ScannedFile) ScanSinkControl
}
type ScanSinkFunc func(*ScannedFile) ScanSinkControl

func (f ScanSinkFunc) OnFile(file *ScannedFile) ScanSinkControl { return f(file) }

type ScanStreamReport struct {
	Root              string
	Selected, Emitted uint64
	Stopped           bool
	Skipped           []SkippedEntry
	Warnings          []ScanWarning
	IgnoreSources     []IgnoreSourceEvidence
	Revision          string
	Complete          bool
	Termination       ScanTermination
	Portable          bool
	Cache             ScanCacheStats
}

func (s *Scanner) ScanInto(ctx context.Context, sink ScanSink) (*ScanStreamReport, error) {
	inner, err := scan.Into(ctx, s.root, toScanOptions(s.options), func(file *scan.ScannedFile) scan.StreamControl {
		pub := ScannedFile{Absolute: file.Absolute, Relative: file.Relative, Bytes: file.Bytes, ContentHash: file.ContentHash, ContentFingerprint: file.ContentFingerprint, Version: file.Version, BinaryChecked: file.BinaryChecked}
		if sink.OnFile(&pub) == ScanSinkStop {
			return scan.SinkStop
		}
		return scan.SinkContinue
	})
	if err != nil {
		return nil, wrap(err, "ScanInto", s.root)
	}
	return &ScanStreamReport{
		Root: inner.Root, Selected: inner.Selected, Emitted: inner.Emitted, Stopped: inner.Stopped,
		Revision: inner.Revision, Complete: inner.Complete, Termination: ScanTermination(inner.Termination),
		Portable: inner.Portable, Cache: ScanCacheStats(inner.Cache),
		Skipped: copySkipped(inner.Skipped), Warnings: copyWarnings(inner.Warnings), IgnoreSources: copySources(inner.IgnoreSources),
	}, nil
}

type WatchEventKind int

const (
	WatchCreate WatchEventKind = iota
	WatchModify
	WatchRemove
	WatchRenameFrom
	WatchRenameTo
	WatchDirectory
	WatchRescan
)

type WatchEvent struct {
	Path string
	Kind WatchEventKind
}

func NewWatchEvent(path string, kind WatchEventKind) WatchEvent {
	return WatchEvent{Path: path, Kind: kind}
}

type WatchPlan struct {
	Changed, Removed []string
	FullRescan       bool
	RejectedEvents   uint64
}

func (p WatchPlan) Invalidated() []string {
	return append(append([]string(nil), p.Changed...), p.Removed...)
}
func (p WatchPlan) InvalidatesPath(relative string) bool {
	for _, prefix := range p.Invalidated() {
		if pathx.IsSameOrDescendant(relative, prefix) {
			return true
		}
	}
	return false
}
func (p WatchPlan) ExpandRemoved(known []string) []string {
	prefixes := pathx.CollapsePathPrefixes(p.Removed)
	var out []string
	for _, path := range known {
		if pathx.PathCoveredByPrefixes(path, prefixes) {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

type WatchUpdateReason int

const (
	WatchReasonIncremental WatchUpdateReason = iota
	WatchReasonFullPolicy
	WatchReasonFullIgnore
	WatchReasonFullStructural
	WatchReasonFullIncomplete
)

func (r WatchUpdateReason) String() string {
	names := [...]string{"Incremental", "FullRescan:PolicyChanged", "FullRescan:IgnoreInputChanged", "FullRescan:StructuralChange", "FullRescan:IncompletePreviousState"}
	if int(r) >= 0 && int(r) < len(names) {
		return names[r]
	}
	return ""
}
func (r WatchUpdateReason) AsString() string { return r.String() }

type FullRescanReason int

const (
	FullRescanPolicyChanged FullRescanReason = iota
	FullRescanIgnoreInputChanged
	FullRescanStructuralChange
	FullRescanIncompletePreviousState
)

type WatchUpdate struct {
	Report *ScanReport
	Reason WatchUpdateReason
}

func watchReason(r scan.WatchReason) WatchUpdateReason { return WatchUpdateReason(r) }

type WatcherEventAdapter struct {
	root, eventRoot string
	ignoreName      map[string]struct{}
}

func NewWatcherEventAdapter(root string, ignoreFiles []string) (*WatcherEventAdapter, error) {
	eventRoot := root
	if !filepath.IsAbs(root) {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		eventRoot = filepath.Join(cwd, root)
	}
	canonical, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &Error{Code: CodeInvalid, Op: "NewWatcherEventAdapter", Path: canonical, Err: errString("root is not a directory")}
	}
	names := map[string]struct{}{}
	for _, name := range ignoreFiles {
		names[filepath.Base(strings.ReplaceAll(name, "\\", "/"))] = struct{}{}
	}
	return &WatcherEventAdapter{root: canonical, eventRoot: eventRoot, ignoreName: names}, nil
}

func NewWatcherEventAdapterWithOptions(root string, opts Options) (*WatcherEventAdapter, error) {
	return NewWatcherEventAdapter(root, opts.IgnoreFiles)
}

func (a *WatcherEventAdapter) Plan(events []WatchEvent) WatchPlan {
	changed, removed := map[string]struct{}{}, map[string]struct{}{}
	var plan WatchPlan
	for _, event := range events {
		if event.Kind == WatchRescan {
			plan.FullRescan = true
			continue
		}
		rel, ok := a.relative(event.Path)
		if !ok {
			plan.RejectedEvents++
			continue
		}
		if rel == "" || event.Kind == WatchDirectory || a.controls(rel) {
			plan.FullRescan = true
			continue
		}
		switch event.Kind {
		case WatchCreate, WatchModify, WatchRenameTo:
			delete(removed, rel)
			changed[rel] = struct{}{}
		case WatchRemove, WatchRenameFrom:
			delete(changed, rel)
			removed[rel] = struct{}{}
		}
	}
	for rel := range changed {
		plan.Changed = append(plan.Changed, rel)
	}
	for rel := range removed {
		plan.Removed = append(plan.Removed, rel)
	}
	sort.Strings(plan.Changed)
	sort.Strings(plan.Removed)
	return plan
}

func (a *WatcherEventAdapter) relative(path string) (string, bool) {
	rel := path
	if filepath.IsAbs(path) {
		got, err := filepath.Rel(a.eventRoot, path)
		if err != nil {
			got, err = filepath.Rel(a.root, path)
		}
		if err != nil {
			return "", false
		}
		rel = got
	}
	rel = filepath.ToSlash(rel)
	if strings.Contains(rel, "..") || strings.HasPrefix(rel, "/") {
		return "", false
	}
	if rel == "." {
		return "", true
	}
	return rel, true
}

func (a *WatcherEventAdapter) controls(rel string) bool {
	if rel == ".git/config" || rel == ".git/info/exclude" {
		return true
	}
	_, ok := a.ignoreName[filepath.Base(rel)]
	return ok
}

type DeltaQuality int

const (
	DeltaContentHash DeltaQuality = iota
	DeltaMetadata
	DeltaPartial
)

type ModifiedFile struct{ Previous, Current ScannedFile }
type RenamedFile struct{ Previous, Current ScannedFile }

type ScanDelta struct {
	FromRevision, ToRevision                                string
	Added, Removed                                          []ScannedFile
	Modified                                                []ModifiedFile
	Renamed                                                 []RenamedFile
	Unchanged                                               uint64
	SelectionInputsChanged, PolicyChanged, ScanStateChanged bool
	Quality                                                 DeltaQuality
}

func (d ScanDelta) IsEmpty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Modified) == 0 && len(d.Renamed) == 0 && !d.SelectionInputsChanged && !d.PolicyChanged && !d.ScanStateChanged
}

func DeltaBetween(previous, current *ScanReport) ScanDelta {
	quality := deltaQuality(previous, current)
	delta := ScanDelta{
		FromRevision: previous.Revision, ToRevision: current.Revision, Quality: quality,
		SelectionInputsChanged: ignoreSourcesDiffer(previous.IgnoreSources, current.IgnoreSources),
		PolicyChanged:          previous.Descriptor != current.Descriptor,
		ScanStateChanged:       previous.Root != current.Root || previous.Complete != current.Complete || previous.Termination != current.Termination || previous.Portable != current.Portable,
	}
	prevBy, curBy := indexFiles(previous.Files), indexFiles(current.Files)
	inner := report.Between(toRecords(previous.Files), toRecords(current.Files))
	delta.Unchanged = inner.Unchanged
	delta.Added = fromRecords(inner.Added, curBy)
	delta.Removed = fromRecords(inner.Removed, prevBy)
	for _, ch := range inner.Modified {
		delta.Modified = append(delta.Modified, ModifiedFile{Previous: prevBy[ch.Previous.Relative], Current: curBy[ch.Current.Relative]})
	}
	for _, ch := range inner.Renamed {
		delta.Renamed = append(delta.Renamed, RenamedFile{Previous: prevBy[ch.Previous.Relative], Current: curBy[ch.Current.Relative]})
	}
	return delta
}

func toRecords(files []ScannedFile) []report.Record {
	out := make([]report.Record, len(files))
	for i, file := range files {
		key := file.ContentHash
		if key == "" && file.Version.ModifiedNS != nil {
			key = fmt.Sprintf("%d", *file.Version.ModifiedNS)
		}
		out[i] = report.Record{Relative: file.Relative, ContentHash: file.ContentHash, VersionKey: key, Bytes: file.Bytes}
	}
	return out
}

func indexFiles(files []ScannedFile) map[string]ScannedFile {
	out := make(map[string]ScannedFile, len(files))
	for _, file := range files {
		out[file.Relative] = file
	}
	return out
}

func fromRecords(records []report.Record, byRel map[string]ScannedFile) []ScannedFile {
	out := make([]ScannedFile, 0, len(records))
	for _, rec := range records {
		out = append(out, byRel[rec.Relative])
	}
	return out
}

func deltaQuality(previous, current *ScanReport) DeltaQuality {
	if !previous.Complete || !current.Complete || previous.Termination != TerminationNone || current.Termination != TerminationNone {
		return DeltaPartial
	}
	for _, file := range append(append([]ScannedFile(nil), previous.Files...), current.Files...) {
		if file.ContentHash == "" {
			return DeltaMetadata
		}
	}
	return DeltaContentHash
}

func ignoreSourcesDiffer(a, b []IgnoreSourceEvidence) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

type (
	PortableScannedFile struct {
		Relative      string `json:"relative"`
		Bytes         uint64 `json:"bytes"`
		ContentHash   string `json:"content_hash,omitempty"`
		BinaryChecked bool   `json:"binary_checked"`
	}
	PortableSkippedEntry struct {
		Relative   string   `json:"relative"`
		Kind       SkipKind `json:"kind"`
		DetailHash string   `json:"detail_hash,omitempty"`
	}
	PortableScanWarning struct {
		Relative    string `json:"relative,omitempty"`
		MessageHash string `json:"message_hash"`
	}
	PortableIgnoreSourceEvidence struct {
		Kind               IgnoreSourceKind `json:"kind"`
		RepositoryRelative string           `json:"repository_relative,omitempty"`
		ContentHash        string           `json:"content_hash"`
	}
	PortableScanReport struct {
		Files             []PortableScannedFile          `json:"files"`
		Skipped           []PortableSkippedEntry         `json:"skipped"`
		Warnings          []PortableScanWarning          `json:"warnings"`
		IgnoreSources     []PortableIgnoreSourceEvidence `json:"ignore_sources"`
		Revision          string                         `json:"revision"`
		Descriptor        ScanDescriptor                 `json:"descriptor"`
		Complete          bool                           `json:"complete"`
		Termination       ScanTermination                `json:"termination,omitempty"`
		SelectionPortable bool                           `json:"selection_portable"`
	}
)

func (r *ScanReport) ToPortable() PortableScanReport {
	out := PortableScanReport{Descriptor: r.Descriptor, Complete: r.Complete, Termination: r.Termination, SelectionPortable: r.Portable, Revision: r.Revision}
	for _, file := range r.Files {
		out.Files = append(out.Files, PortableScannedFile{Relative: file.Relative, Bytes: file.Bytes, ContentHash: file.ContentHash, BinaryChecked: file.BinaryChecked})
	}
	for _, skipped := range r.Skipped {
		item := PortableSkippedEntry{Relative: skipped.Relative, Kind: skipped.Kind}
		if skipped.Detail != "" {
			item.DetailHash = report.HashText(skipped.Detail)
		}
		out.Skipped = append(out.Skipped, item)
	}
	for _, warning := range r.Warnings {
		out.Warnings = append(out.Warnings, PortableScanWarning{Relative: warning.Relative, MessageHash: report.HashText(warning.Message)})
	}
	for _, src := range r.IgnoreSources {
		out.IgnoreSources = append(out.IgnoreSources, PortableIgnoreSourceEvidence{Kind: src.Kind, RepositoryRelative: report.RepoRelativeLocation(src.Location), ContentHash: src.ContentHash})
	}
	sort.Slice(out.IgnoreSources, func(i, j int) bool {
		if out.IgnoreSources[i].Kind != out.IgnoreSources[j].Kind {
			return out.IgnoreSources[i].Kind < out.IgnoreSources[j].Kind
		}
		return out.IgnoreSources[i].RepositoryRelative < out.IgnoreSources[j].RepositoryRelative
	})
	return out
}

func (r *ScanReport) PortableFilesPage(offset, limit int) []PortableScannedFile {
	if offset >= len(r.Files) {
		return nil
	}
	end := offset + limit
	if end > len(r.Files) {
		end = len(r.Files)
	}
	out := make([]PortableScannedFile, 0, end-offset)
	for _, file := range r.Files[offset:end] {
		out = append(out, PortableScannedFile{Relative: file.Relative, Bytes: file.Bytes, ContentHash: file.ContentHash, BinaryChecked: file.BinaryChecked})
	}
	return out
}

type PathExplanation struct {
	Relative, Outcome, Reason, Source, Pattern string
	Line                                       int
}

// Explain reports the winning selection rule for rel.
// It does not hash the file or apply binary / max-file-bytes checks.
func (s *Scanner) Explain(rel string) (PathExplanation, error) {
	if s == nil {
		return PathExplanation{}, &Error{Code: CodeInvalid, Op: "Explain", Err: errEmptyRoot}
	}
	m, err := NewSelectionMatcher(s.root, s.options)
	if err != nil {
		return PathExplanation{}, wrap(err, "Explain", s.root)
	}
	rel = pathx.Slash(rel)
	_ = m.LoadPathScope(rel)
	info, statErr := os.Lstat(filepath.Join(m.Root(), filepath.FromSlash(rel)))
	x := m.Explain(rel, statErr == nil && info.IsDir())
	return PathExplanation{x.Relative, x.Outcome, x.Reason, x.Source, x.Pattern, x.Line}, nil
}
