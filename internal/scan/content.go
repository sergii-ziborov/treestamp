package scan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/sergii-ziborov/treestamp/internal/hashx"
	"github.com/sergii-ziborov/treestamp/internal/selection"
)

type ContentVisitMode int

const (
	VisitRevision ContentVisitMode = iota
	VisitStreaming
)

type ContentVisitControl int

const (
	ContentContinue ContentVisitControl = iota
	ContentSkipFile
	ContentQuit
)

type ContentFile struct {
	RootIndex uint
	Sequence  uint64
	Root      string
	Absolute  string
	Relative  string
	Bytes     uint64
}

type ContentFileStatus int

const (
	ContentSelected ContentFileStatus = iota
	ContentBinary
	ContentChanged
)

type ContentVisitKind int

const (
	ContentFileStart ContentVisitKind = iota
	ContentChunk
	ContentFileEnd
)

type ContentVisitEvent struct {
	Kind            ContentVisitKind
	WorkerIndex     int
	File            ContentFile
	Offset          uint64
	Bytes           []byte
	Status          ContentFileStatus
	BytesRead       uint64
	ContentHash     string
	ConsumerSkipped bool
}

type ContentVisitor func(ContentVisitEvent) ContentVisitControl

type ContentVisitReport struct {
	Mode            ContentVisitMode
	Root            string
	Discovered      uint64
	Completed       uint64
	Opened          uint64
	Chunks          uint64
	BytesRead       uint64
	BytesEmitted    uint64
	ConsumerSkipped uint64
	Stopped         bool
	Skipped         []Skipped
	Warnings        []Warning
	IgnoreSources   []IgnoreSource
	Revision        string
	Complete        bool
	Termination     Termination
	Portable        bool
	Cache           CacheStats
	Manifest        *CompactReport
}

func VisitContent(ctx context.Context, root string, opts Options, mode ContentVisitMode, factory func(worker int) ContentVisitor) (*ContentVisitReport, error) {
	discovered, err := discover(ctx, root, opts, true)
	if err != nil {
		return nil, err
	}
	opts.Root = discovered.root
	return runVisit(ctx, opts, mode, factory, discovered)
}

func VisitChanged(ctx context.Context, root string, opts Options, plan WatchPlan, factory func(worker int) ContentVisitor) (*ContentVisitReport, error) {
	discovered, err := discoverChanged(ctx, root, opts, plan)
	if err != nil {
		return nil, err
	}
	opts.Root = discovered.root
	return runVisit(ctx, opts, VisitRevision, factory, discovered)
}

func discoverChanged(ctx context.Context, root string, opts Options, plan WatchPlan) (*discovery, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	matcher, err := selection.NewMatcher(abs, selectionConfig(opts, opts.Walk, true))
	if err != nil {
		return nil, err
	}
	rejected := rejectedRels(plan.Changed)
	plan = sanitizePlan(plan)
	scope := watchScope{ctx: ctx, root: root, abs: abs, opts: opts, matcher: matcher, plan: plan}
	set, err := collectChanges(scope, false)
	if err != nil {
		return nil, err
	}
	for _, rel := range rejected {
		set.skipped = append(set.skipped, Skipped{Relative: rel, Kind: selection.SkipPathEscape})
	}
	return &discovery{
		root: abs, candidates: set.candidates, skipped: set.skipped,
		sources: toIgnoreSources(matcher.Sources()), complete: true, portable: portablePolicy(opts),
	}, nil
}

func runVisit(ctx context.Context, opts Options, mode ContentVisitMode, factory func(worker int) ContentVisitor, discovered *discovery) (*ContentVisitReport, error) {
	visitor := factory(0)
	report := newVisitReport(discovered, mode)
	var files []ScannedFile
	for i, c := range discovered.candidates {
		if err := ctx.Err(); err != nil {
			report.Termination = TermCancelled
			report.Complete = false
			break
		}
		file := ContentFile{Sequence: uint64(i + 1), Root: discovered.root, Absolute: c.abs, Relative: c.rel, Bytes: c.size}
		start := visitor(ContentVisitEvent{Kind: ContentFileStart, File: file})
		if start == ContentSkipFile {
			report.ConsumerSkipped++
			continue
		}
		if start == ContentQuit {
			report.Stopped = true
			break
		}
		if quit := accumulateVisit(report, &files, visitWork{ctx: ctx, c: c, file: file, opts: opts, visitor: visitor}); quit {
			report.Stopped = true
			break
		}
	}
	if mode == VisitRevision {
		attachVisitManifest(report, discovered, files, opts)
	}
	return report, nil
}

func newVisitReport(discovered *discovery, mode ContentVisitMode) *ContentVisitReport {
	return &ContentVisitReport{
		Mode: mode, Root: discovered.root, Discovered: uint64(len(discovered.candidates)),
		Skipped: discovered.skipped, Warnings: discovered.warnings, IgnoreSources: discovered.sources,
		Complete: discovered.complete, Termination: discovered.term, Portable: discovered.portable,
	}
}

type visitWork struct {
	ctx     context.Context
	c       candidate
	file    ContentFile
	opts    Options
	visitor ContentVisitor
}

func accumulateVisit(report *ContentVisitReport, files *[]ScannedFile, work visitWork) bool {
	scanned, skip, stat, ev, status, consumerSkip, quit, opened, committed := readVisited(work)
	if opened {
		report.Opened++
	}
	report.Chunks += ev.chunks
	report.BytesRead += ev.bytesRead
	report.BytesEmitted += ev.bytesEmitted
	report.Cache.ContentReads += stat.ContentReads
	report.Cache.ReusedHashes += stat.ReusedHashes
	if consumerSkip {
		report.ConsumerSkipped++
	}
	if skip != nil {
		report.Skipped = append(report.Skipped, *skip)
	} else if committed && status == ContentSelected {
		report.Completed++
		*files = append(*files, scanned)
	}
	return quit
}

func attachVisitManifest(report *ContentVisitReport, discovered *discovery, files []ScannedFile, opts Options) {
	full := &Report{
		Root: discovered.root, Files: files, Skipped: report.Skipped, Warnings: report.Warnings,
		IgnoreSources: report.IgnoreSources, Complete: report.Complete && !report.Stopped,
		Termination: report.Termination, Portable: report.Portable, Cache: report.Cache,
	}
	finalize(full, opts)
	report.Revision = full.Revision
	compact := &CompactReport{
		Root: full.Root, Skipped: full.Skipped, Warnings: full.Warnings, IgnoreSources: full.IgnoreSources,
		Revision: full.Revision, Descriptor: full.Descriptor, Complete: full.Complete,
		Termination: full.Termination, Portable: full.Portable, Cache: full.Cache,
	}
	for _, f := range full.Files {
		compact.Files = append(compact.Files, CompactFile{
			Relative: f.Relative, Bytes: f.Bytes,
			Content: &ContentEvidence{ContentHash: f.ContentHash, ContentFingerprint: f.ContentFingerprint, Version: f.Version, BinaryChecked: f.BinaryChecked},
		})
	}
	report.Manifest = compact
}

type visitCounters struct {
	chunks, bytesRead, bytesEmitted uint64
}

func readVisited(work visitWork) (ScannedFile, *Skipped, CacheStats, visitCounters, ContentFileStatus, bool, bool, bool, bool) {
	c := work.c
	scanned := ScannedFile{Absolute: c.abs, Relative: c.rel, Bytes: c.size, Version: c.version}
	path, skip := confineCandidate(c, work.opts)
	if skip != nil {
		return scanned, skip, CacheStats{}, visitCounters{}, ContentChanged, false, false, false, false
	}
	f, err := os.Open(path)
	if err != nil {
		return scanned, &Skipped{Relative: c.rel, Kind: selection.SkipIOError, Detail: err.Error()}, CacheStats{}, visitCounters{}, ContentChanged, false, false, false, false
	}
	defer f.Close()
	scanned, skip, stat, ev, status, consumer, quit, committed := finishVisited(f, scanned, work)
	return scanned, skip, stat, ev, status, consumer, quit, true, committed
}

func finishVisited(f *os.File, scanned ScannedFile, work visitWork) (ScannedFile, *Skipped, CacheStats, visitCounters, ContentFileStatus, bool, bool, bool) {
	ctx, file, opts, visitor := work.ctx, work.file, work.opts, work.visitor
	h := sha256.New()
	fp := hashx.NewContentFingerprint()
	buf := make([]byte, 64*1024)
	var offset uint64
	var counters visitCounters
	skipRest, consumerSkip := false, false
	for {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return scanned, &Skipped{Relative: scanned.Relative, Kind: selection.SkipIOError, Detail: err.Error()}, CacheStats{ContentReads: 1}, counters, ContentChanged, consumerSkip, true, false
			}
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			skip, quit, binary := emitVisitedChunk(chunkEmit{
				chunk: buf[:n], scanned: &scanned, file: file, opts: opts, visitor: visitor,
				h: h, fp: &fp, offset: &offset, counters: &counters, skipRest: &skipRest, consumerSkip: &consumerSkip,
			})
			if binary {
				return scanned, skip, CacheStats{ContentReads: 1}, counters, ContentBinary, consumerSkip, false, false
			}
			if quit {
				return scanned, nil, CacheStats{ContentReads: 1}, counters, ContentChanged, consumerSkip, true, false
			}
			if skip != nil {
				return scanned, skip, CacheStats{ContentReads: 1}, counters, ContentChanged, consumerSkip, false, false
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return scanned, &Skipped{Relative: scanned.Relative, Kind: selection.SkipIOError, Detail: readErr.Error()}, CacheStats{ContentReads: 1}, counters, ContentChanged, consumerSkip, false, false
		}
	}
	if skip := checkContentSize(f, candidate{rel: scanned.Relative, size: scanned.Bytes, version: scanned.Version}, opts); skip != nil {
		return scanned, skip, CacheStats{ContentReads: 1}, counters, ContentChanged, consumerSkip, false, false
	}
	if opts.HashFileContents {
		scanned.ContentHash = hashx.Finish(h)
	}
	scanned.ContentFingerprint = fp.Finish()
	scanned.BinaryChecked = opts.DetectBinary
	end := ContentVisitEvent{Kind: ContentFileEnd, File: file, Status: ContentSelected, BytesRead: counters.bytesRead, ContentHash: scanned.ContentHash, ConsumerSkipped: consumerSkip}
	return scanned, nil, CacheStats{ContentReads: 1}, counters, ContentSelected, consumerSkip, visitor(end) == ContentQuit, true
}

type chunkEmit struct {
	chunk                  []byte
	scanned                *ScannedFile
	file                   ContentFile
	opts                   Options
	visitor                ContentVisitor
	h                      hash.Hash
	fp                     *hashx.ContentFingerprint
	offset                 *uint64
	counters               *visitCounters
	skipRest, consumerSkip *bool
}

func emitVisitedChunk(e chunkEmit) (*Skipped, bool, bool) {
	e.counters.bytesRead += uint64(len(e.chunk))
	if e.opts.DetectBinary && bytes.IndexByte(e.chunk, 0) >= 0 {
		e.visitor(ContentVisitEvent{Kind: ContentFileEnd, File: e.file, Status: ContentBinary, BytesRead: e.counters.bytesRead, ConsumerSkipped: *e.consumerSkip})
		return &Skipped{Relative: e.scanned.Relative, Kind: selection.SkipBinary}, false, true
	}
	_, _ = e.h.Write(e.chunk)
	e.fp.Write(e.chunk)
	if !*e.skipRest {
		control := e.visitor(ContentVisitEvent{Kind: ContentChunk, File: e.file, Offset: *e.offset, Bytes: e.chunk})
		e.counters.chunks++
		e.counters.bytesEmitted += uint64(len(e.chunk))
		if control == ContentSkipFile {
			*e.skipRest, *e.consumerSkip = true, true
		}
		if control == ContentQuit {
			return nil, true, false
		}
	}
	*e.offset += uint64(len(e.chunk))
	return nil, false, false
}

type StreamControl int

const (
	SinkContinue StreamControl = iota
	SinkStop
)

type StreamReport struct {
	Root          string
	Selected      uint64
	Emitted       uint64
	Stopped       bool
	Skipped       []Skipped
	Warnings      []Warning
	IgnoreSources []IgnoreSource
	Revision      string
	Complete      bool
	Termination   Termination
	Portable      bool
	Cache         CacheStats
}

func Into(ctx context.Context, root string, opts Options, sink func(*ScannedFile) StreamControl) (*StreamReport, error) {
	discovered, err := discover(ctx, root, opts, true)
	if err != nil {
		return nil, err
	}
	opts.Root = discovered.root
	sort.Slice(discovered.candidates, func(i, j int) bool { return discovered.candidates[i].rel < discovered.candidates[j].rel })
	index := cacheIndex(opts)
	out := &StreamReport{
		Root: discovered.root, Skipped: append([]Skipped(nil), discovered.skipped...),
		Warnings: discovered.warnings, IgnoreSources: discovered.sources,
		Complete: discovered.complete, Termination: discovered.term, Portable: discovered.portable,
	}
	rev := newRevisionBuilder(discovered.sources)
	for _, c := range discovered.candidates {
		if err := ctx.Err(); err != nil {
			out.Termination = TermCancelled
			out.Complete = false
			break
		}
		file, skip, stat, inspectErr := inspectOne(ctx, c, opts, index, newContentMemo())
		if inspectErr != nil {
			return nil, inspectErr
		}
		out.Cache.ReusedHashes += stat.ReusedHashes
		out.Cache.ContentReads += stat.ContentReads
		out.Cache.FingerprintReads += stat.FingerprintReads
		if skip != nil {
			out.Skipped = append(out.Skipped, *skip)
			continue
		}
		rev.push(file)
		out.Selected++
		out.Emitted++
		if sink(&file) == SinkStop {
			out.Stopped = true
			out.Complete = false
			break
		}
	}
	out.Revision = rev.finish(out.Portable, out.Termination)
	return out, nil
}
