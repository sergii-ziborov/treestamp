package scan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"hash"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"

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
	opts.ensureStarted()
	discovered, err := discover(ctx, root, opts, true)
	if err != nil {
		return nil, err
	}
	opts.Root = discovered.root
	return runVisit(ctx, opts, mode, factory, discovered)
}

func VisitChanged(ctx context.Context, root string, opts Options, plan WatchPlan, factory func(worker int) ContentVisitor) (*ContentVisitReport, error) {
	opts.ensureStarted()
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
	report := newVisitReport(discovered, mode)
	workers := opts.ContentWorkers
	if workers < 1 {
		workers = 1
	}
	var files []ScannedFile
	retain := (*[]ScannedFile)(nil)
	if mode == VisitRevision {
		retain = &files
	}
	if workers == 1 {
		runVisitLoop(ctx, report, discovered, opts, factory(0), retain)
	} else {
		runVisitParallel(ctx, report, discovered, opts, factory, workers, retain)
	}
	if mode == VisitRevision {
		sort.Slice(files, func(i, j int) bool { return files[i].Relative < files[j].Relative })
		attachVisitManifest(report, discovered, files, opts)
	}
	return report, nil
}

func runVisitLoop(ctx context.Context, report *ContentVisitReport, discovered *discovery, opts Options, visitor ContentVisitor, files *[]ScannedFile) {
	for i, c := range discovered.candidates {
		if err := ctx.Err(); err != nil {
			report.Termination, report.Complete = TermCancelled, false
			return
		}
		if scanTimedOut(opts) {
			report.Termination, report.Complete = TermTimeout, false
			return
		}
		file := ContentFile{Sequence: uint64(i + 1), Root: discovered.root, Absolute: c.abs, Relative: c.rel, Bytes: c.size}
		start := visitor(ContentVisitEvent{Kind: ContentFileStart, File: file})
		if start == ContentSkipFile {
			report.ConsumerSkipped++
			continue
		}
		if start == ContentQuit {
			report.Stopped = true
			return
		}
		if accumulateVisit(report, files, visitWork{ctx: ctx, c: c, file: file, opts: opts, visitor: visitor}) {
			report.Stopped = true
			return
		}
	}
}

func runVisitParallel(ctx context.Context, report *ContentVisitReport, discovered *discovery, opts Options, factory func(int) ContentVisitor, workers int, files *[]ScannedFile) {
	var mu sync.Mutex
	var stop atomic.Bool
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(visitor ContentVisitor) {
			defer wg.Done()
			for i := range jobs {
				if stop.Load() {
					continue
				}
				if haltVisit(ctx, opts, report, &mu, &stop) {
					return
				}
				c := discovered.candidates[i]
				file := ContentFile{Sequence: uint64(i + 1), Root: discovered.root, Absolute: c.abs, Relative: c.rel, Bytes: c.size}
				start := visitor(ContentVisitEvent{Kind: ContentFileStart, File: file})
				if start == ContentSkipFile {
					mu.Lock()
					report.ConsumerSkipped++
					mu.Unlock()
					continue
				}
				if start == ContentQuit {
					mu.Lock()
					report.Stopped = true
					stop.Store(true)
					mu.Unlock()
					return
				}
				work := visitWork{ctx: ctx, c: c, file: file, opts: opts, visitor: visitor}
				scanned, skip, stat, ev, status, consumerSkip, quit, opened, committed := readVisited(work)
				mu.Lock()
				mergeVisit(report, files, scanned, skip, stat, ev, status, consumerSkip, opened, committed)
				if quit {
					report.Stopped = true
					stop.Store(true)
				}
				mu.Unlock()
			}
		}(factory(w))
	}
	for i := range discovered.candidates {
		if stop.Load() {
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()
}

func haltVisit(ctx context.Context, opts Options, report *ContentVisitReport, mu *sync.Mutex, stop *atomic.Bool) bool {
	if ctx.Err() == nil && !scanTimedOut(opts) {
		return false
	}
	mu.Lock()
	if ctx.Err() != nil {
		report.Termination, report.Complete = TermCancelled, false
	} else {
		report.Termination, report.Complete = TermTimeout, false
	}
	stop.Store(true)
	mu.Unlock()
	return true
}

func mergeVisit(report *ContentVisitReport, files *[]ScannedFile, scanned ScannedFile, skip *Skipped, stat CacheStats, ev visitCounters, status ContentFileStatus, consumerSkip, opened, committed bool) {
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
		if files != nil {
			*files = append(*files, scanned)
		}
	}
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
	mergeVisit(report, files, scanned, skip, stat, ev, status, consumerSkip, opened, committed)
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
	budget := contentBudget(scanned.Bytes, opts)
	for {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return scanned, &Skipped{Relative: scanned.Relative, Kind: selection.SkipIOError, Detail: err.Error()}, CacheStats{ContentReads: 1}, counters, ContentChanged, consumerSkip, true, false
			}
		}
		chunk, skip, done, _ := readBudgeted(f, buf, offset, budget, scanned.Relative)
		if skip != nil {
			return scanned, skip, CacheStats{ContentReads: 1}, counters, ContentChanged, consumerSkip, false, false
		}
		if len(chunk) > 0 {
			skip, quit, binary := emitVisitedChunk(chunkEmit{
				chunk: chunk, scanned: &scanned, file: file, opts: opts, visitor: visitor,
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
		if done {
			break
		}
	}
	if opts.ContentValidation == ContentStrict && scanned.Bytes > 0 && offset < scanned.Bytes {
		return scanned, &Skipped{Relative: scanned.Relative, Kind: selection.SkipConcurrentModification}, CacheStats{ContentReads: 1}, counters, ContentChanged, consumerSkip, false, false
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

func VisitOwnedFS(ctx context.Context, fsys fs.FS, root string, opts Options, consume func(ScannedFile, []byte) error) (*ContentVisitReport, error) {
	discovered, err := discoverFS(ctx, fsys, root, opts, true)
	if err != nil {
		return nil, err
	}
	opts.Root, opts.Cache = "", nil
	report := newVisitReport(discovered, VisitRevision)
	var files []ScannedFile
	for _, c := range discovered.candidates {
		if err := ctx.Err(); err != nil {
			report.Termination, report.Complete = TermCancelled, false
			break
		}
		file, skip, stat, err := ownedOneFS(ctx, fsys, c, opts, consume)
		report.Cache.ContentReads += stat.ContentReads
		report.Cache.ReusedHashes += stat.ReusedHashes
		if err != nil {
			return report, err
		}
		if skip != nil {
			report.Skipped = append(report.Skipped, *skip)
			continue
		}
		report.Completed++
		files = append(files, file)
	}
	attachVisitManifest(report, discovered, files, opts)
	return report, nil
}

func ownedOneFS(ctx context.Context, fsys fs.FS, c candidate, opts Options, consume func(ScannedFile, []byte) error) (ScannedFile, *Skipped, CacheStats, error) {
	file := ScannedFile{Absolute: c.abs, Relative: c.rel, Bytes: c.size}
	f, err := fsys.Open(c.abs)
	if err != nil {
		return file, &Skipped{Relative: c.rel, Kind: selection.SkipIOError, Detail: err.Error()}, CacheStats{}, nil
	}
	defer f.Close()
	file, data, skip, stat, err := hashReader(ctx, f, c, opts, file, true)
	if err != nil || skip != nil || consume == nil {
		return file, skip, stat, err
	}
	return file, skip, stat, consume(file, data)
}

func VisitOwned(ctx context.Context, root string, opts Options, consume func(ScannedFile, []byte) error) (*ContentVisitReport, error) {
	discovered, err := discover(ctx, root, opts, true)
	if err != nil {
		return nil, err
	}
	opts.Root = discovered.root
	report := newVisitReport(discovered, VisitRevision)
	index := cacheIndex(opts)
	memo := newContentMemo()
	var files []ScannedFile
	for _, c := range discovered.candidates {
		if err := ctx.Err(); err != nil {
			report.Termination, report.Complete = TermCancelled, false
			break
		}
		file, skip, stat, err := ownedOne(ctx, c, opts, index, memo, consume)
		report.Cache.ContentReads += stat.ContentReads
		report.Cache.ReusedHashes += stat.ReusedHashes
		if err != nil {
			return report, err
		}
		if skip != nil {
			report.Skipped = append(report.Skipped, *skip)
			continue
		}
		report.Completed++
		files = append(files, file)
	}
	attachVisitManifest(report, discovered, files, opts)
	return report, nil
}

func ownedOne(ctx context.Context, c candidate, opts Options, index map[string]CacheEntry, memo *contentMemo, consume func(ScannedFile, []byte) error) (ScannedFile, *Skipped, CacheStats, error) {
	file, data, skip, stat, err := readOwned(ctx, c, opts, index, memo)
	if err == nil && skip != nil && skip.Kind == selection.SkipConcurrentModification && ctx.Err() == nil {
		file, data, skip, stat, err = readOwned(ctx, c, opts, index, memo)
	}
	if err != nil || skip != nil || consume == nil {
		return file, skip, stat, err
	}
	return file, skip, stat, consume(file, data)
}

func readOwned(ctx context.Context, c candidate, opts Options, _ map[string]CacheEntry, memo *contentMemo) (ScannedFile, []byte, *Skipped, CacheStats, error) {
	file := ScannedFile{Absolute: c.abs, Relative: c.rel, Bytes: c.size, Version: c.version}
	file, data, skip, stat, err := hashOpened(ctx, c, opts, file, true, memo)
	if err == nil && skip == nil {
		memo.store(file)
	}
	return file, data, skip, stat, err
}
