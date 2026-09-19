package scan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"hash"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"time"

	"github.com/sergii-ziborov/treestamp/internal/filetypes"
	"github.com/sergii-ziborov/treestamp/internal/hashx"
	"github.com/sergii-ziborov/treestamp/internal/ignore"
	"github.com/sergii-ziborov/treestamp/internal/runtime"
	"github.com/sergii-ziborov/treestamp/internal/selection"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

const DescriptorVersion, CacheFormatVersion uint32 = 2, 2

type Options struct {
	MaxFileBytes      uint64
	IgnoreFiles       []string
	OverrideRules     []string
	Extensions        []string
	FileTypes         *filetypes.NamedFileTypes
	IgnorePolicy      ignore.Policy
	IgnoreCase        bool
	SkipHidden        bool
	StandardSkips     bool
	VCSSkips          bool
	GitModules        bool
	Filters           selection.Filters
	HashFileContents  bool
	DetectBinary      bool
	RecordSkipped     bool
	Walk              walk.WalkOptions
	TraversalWorkers  int
	ContentWorkers    int
	Limits            Limits
	Cancel            *runtime.Token
	CacheValidation   CacheValidation
	ContentValidation ContentValidation
	ContentDiscovery  ContentDiscovery
	AdmitTimeout      time.Duration
	Cache             *Cache
	Started           time.Time
	Root              string
	emitPath          func(string) error
	MaxFileBytesZero  bool
}

type Limits struct {
	MaxEntries, MaxTotalBytes *uint64
	Timeout                   time.Duration
}

type CacheValidation int

const (
	CacheFast CacheValidation = iota
	CacheStrict
)

type ContentValidation int

const (
	ContentFast ContentValidation = iota
	ContentStrict
)

type ContentDiscovery int

const (
	DiscoverStreaming ContentDiscovery = iota
	DiscoverBufferedParallel
)

func DefaultOptions() Options {
	w := walk.DefaultOptions()
	w.CollectMetadata = true
	return Options{
		MaxFileBytes: 1_500_000, IgnoreFiles: []string{".gitignore", ".ignore", ".weavatrixignore"},
		IgnorePolicy: ignore.RepositoryPolicy(), StandardSkips: true, HashFileContents: true,
		DetectBinary: true, RecordSkipped: true, Walk: w, CacheValidation: CacheFast,
		ContentValidation: ContentStrict, ContentDiscovery: DiscoverStreaming,
	}
}

type ScannedFile struct {
	Absolute, Relative, ContentHash, ContentFingerprint string
	Bytes                                               uint64
	Version                                             walk.FileVersion
	BinaryChecked                                       bool
}

type CompactFile struct {
	Relative string
	Bytes    uint64
	Content  *ContentEvidence
}

func (f CompactFile) ContentHash() string {
	if f.Content == nil {
		return ""
	}
	return f.Content.ContentHash
}

type ContentEvidence struct {
	ContentHash, ContentFingerprint string
	Version                         walk.FileVersion
	BinaryChecked                   bool
}

type Skipped struct {
	Relative string
	Kind     selection.SkipKind
	Detail   string
}

type Warning struct {
	Relative string
	Message  string
}

type IgnoreSource struct {
	Kind        ignore.SourceKind
	Location    string
	ContentHash string
}

type Descriptor struct {
	Version uint32
	Policy  string
}

type CacheStats struct {
	ReusedHashes, ContentReads, FingerprintReads uint64
	Rebuilt                                      bool
}

type Termination int

const (
	TermNone Termination = iota
	TermMaxEntries
	TermMaxTotalBytes
	TermTimeout
	TermCancelled
)

func (t Termination) String() string {
	switch t {
	case TermMaxEntries:
		return "max_entries"
	case TermMaxTotalBytes:
		return "max_total_bytes"
	case TermTimeout:
		return "timeout"
	case TermCancelled:
		return "cancelled"
	default:
		return ""
	}
}

type Report struct {
	Root          string
	Files         []ScannedFile
	Skipped       []Skipped
	Warnings      []Warning
	IgnoreSources []IgnoreSource
	Revision      string
	Descriptor    Descriptor
	Complete      bool
	Termination   Termination
	Portable      bool
	Cache         CacheStats
}

type CompactReport struct {
	Root          string
	Files         []CompactFile
	Skipped       []Skipped
	Warnings      []Warning
	IgnoreSources []IgnoreSource
	Revision      string
	Descriptor    Descriptor
	Complete      bool
	Termination   Termination
	Portable      bool
	Cache         CacheStats
}

type CacheEntry struct {
	Relative, ContentHash, ContentFingerprint string
	Bytes                                     uint64
	Version                                   walk.FileVersion
	BinaryChecked                             bool
}

type Cache struct {
	FormatVersion uint32
	Root          string
	Entries       []CacheEntry
}

func cacheEntryFrom(rel string, bytes uint64, hash, fp string, ver walk.FileVersion, binary bool) (CacheEntry, bool) {
	if hash == "" || fp == "" {
		return CacheEntry{}, false
	}
	return CacheEntry{Relative: rel, Bytes: bytes, ContentHash: hash, ContentFingerprint: fp, Version: ver, BinaryChecked: binary}, true
}

func CacheFromReport(report *Report) Cache {
	out := Cache{FormatVersion: CacheFormatVersion, Root: report.Root}
	for _, file := range report.Files {
		if e, ok := cacheEntryFrom(file.Relative, file.Bytes, file.ContentHash, file.ContentFingerprint, file.Version, file.BinaryChecked); ok {
			out.Entries = append(out.Entries, e)
		}
	}
	return out
}

func CacheFromCompact(report *CompactReport) Cache {
	out := Cache{FormatVersion: CacheFormatVersion, Root: report.Root}
	for _, file := range report.Files {
		if file.Content == nil {
			continue
		}
		if e, ok := cacheEntryFrom(file.Relative, file.Bytes, file.Content.ContentHash, file.Content.ContentFingerprint, file.Content.Version, file.Content.BinaryChecked); ok {
			out.Entries = append(out.Entries, e)
		}
	}
	return out
}

func (c *Cache) Compatible(root string) bool {
	if c == nil || c.FormatVersion != CacheFormatVersion {
		return false
	}
	if c.Root == root {
		return true
	}
	abs, err := filepath.Abs(root)
	return err == nil && c.Root == abs
}

func (c *Cache) Invalidate(relatives []string) int {
	if c == nil {
		return 0
	}
	before := len(c.Entries)
	keep := c.Entries[:0]
	for _, entry := range c.Entries {
		covered := false
		for _, prefix := range relatives {
			if entry.Relative == prefix || (len(entry.Relative) > len(prefix) && entry.Relative[:len(prefix)] == prefix && entry.Relative[len(prefix)] == '/') {
				covered = true
				break
			}
		}
		if !covered {
			keep = append(keep, entry)
		}
	}
	c.Entries = keep
	return before - len(c.Entries)
}

func Paths(ctx context.Context, root string, opts Options) ([]string, error) {
	opts.ensureStarted()
	discovered, err := discover(ctx, root, pathOpts(opts), false)
	if err != nil {
		return nil, err
	}
	sort.Strings(discovered.paths)
	return discovered.paths, pathTermErr(ctx, discovered)
}

func StreamPaths(ctx context.Context, root string, opts Options, emit func(string) error) error {
	opts.ensureStarted()
	sel := pathOpts(opts)
	sel.emitPath = emit
	if emit == nil {
		return nil
	}
	discovered, err := discover(ctx, root, sel, false)
	if err != nil {
		return err
	}
	if discovered != nil && discovered.emitErr != nil {
		return discovered.emitErr
	}
	return pathTermErr(ctx, discovered)
}

var ErrIncomplete = errText("incomplete selected work")

func pathOpts(opts Options) Options {
	sel := opts
	sel.HashFileContents, sel.DetectBinary, sel.RecordSkipped = false, false, false
	sel.Walk.CollectMetadata = false
	sel.Cache = nil
	return sel
}

func pathTermErr(ctx context.Context, d *discovery) error {
	if d == nil {
		return nil
	}
	if d.term == TermCancelled || d.term == TermTimeout {
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.term == TermTimeout {
			return context.DeadlineExceeded
		}
		return context.Canceled
	}
	if d.term != TermNone || !d.complete {
		return ErrIncomplete
	}
	return nil
}

func (o *Options) ensureStarted() {
	if o != nil && o.Started.IsZero() { o.Started = time.Now() }
}

func Full(ctx context.Context, root string, opts Options) (*Report, error) {
	opts.ensureStarted()
	discovered, err := discover(ctx, root, opts, true)
	if err != nil {
		return nil, err
	}
	opts.Root = discovered.root
	rebuilt := dropBadCache(&opts)
	files, extraSkip, stats, err := inspect(ctx, discovered.candidates, opts)
	if err != nil {
		return nil, err
	}
	stats.Rebuilt = rebuilt
	report := &Report{
		Root: discovered.root, Files: files, Skipped: append(discovered.skipped, extraSkip...),
		Warnings: discovered.warnings, IgnoreSources: discovered.sources, Complete: discovered.complete,
		Termination: discovered.term, Portable: discovered.portable, Cache: stats,
	}
	finalize(report, opts)
	return report, nil
}

func Compact(ctx context.Context, root string, opts Options) (*CompactReport, error) {
	opts.ensureStarted()
	discovered, err := discover(ctx, root, opts, true)
	if err != nil {
		return nil, err
	}
	opts.Root = discovered.root
	rebuilt := dropBadCache(&opts)
	files, extraSkip, stats, err := inspectCompact(ctx, discovered.candidates, opts)
	if err != nil {
		return nil, err
	}
	stats.Rebuilt = rebuilt
	report := &CompactReport{
		Root: discovered.root, Files: files, Skipped: append(discovered.skipped, extraSkip...),
		Warnings: discovered.warnings, IgnoreSources: discovered.sources, Complete: discovered.complete,
		Termination: discovered.term, Portable: discovered.portable, Cache: stats,
	}
	finalizeCompact(report, opts)
	return report, nil
}

func finalizeCompact(report *CompactReport, opts Options) {
	sort.Slice(report.Files, func(i, j int) bool { return report.Files[i].Relative < report.Files[j].Relative })
	sort.Slice(report.Skipped, func(i, j int) bool {
		if report.Skipped[i].Relative != report.Skipped[j].Relative {
			return report.Skipped[i].Relative < report.Skipped[j].Relative
		}
		return report.Skipped[i].Kind < report.Skipped[j].Kind
	})
	sort.Slice(report.Warnings, func(i, j int) bool {
		if report.Warnings[i].Relative != report.Warnings[j].Relative {
			return report.Warnings[i].Relative < report.Warnings[j].Relative
		}
		return report.Warnings[i].Message < report.Warnings[j].Message
	})
	sort.Slice(report.IgnoreSources, func(i, j int) bool {
		if report.IgnoreSources[i].Kind != report.IgnoreSources[j].Kind {
			return report.IgnoreSources[i].Kind < report.IgnoreSources[j].Kind
		}
		return report.IgnoreSources[i].Location < report.IgnoreSources[j].Location
	})
	report.Descriptor = DescriptorFromOptions(opts)
	report.Revision = compactRevision(report.Files, report.IgnoreSources, report.Portable, report.Termination)
}

func Incremental(ctx context.Context, root string, opts Options, previous *Report) (*Report, error) {
	if previous != nil {
		cache := CacheFromReport(previous)
		opts.Cache = &cache
	}
	return Full(ctx, root, opts)
}

func Cached(ctx context.Context, root string, opts Options, cache *Cache) (*Report, error) {
	rebuilt := cache != nil && !cache.Compatible(root)
	if cache != nil && cache.Compatible(root) {
		opts.Cache = cache
	} else {
		opts.Cache = nil
	}
	report, err := Full(ctx, root, opts)
	if report != nil && rebuilt {
		report.Cache.Rebuilt = true
	}
	return report, err
}

func dropBadCache(opts *Options) bool {
	if opts.Cache != nil && !opts.Cache.Compatible(opts.Root) {
		opts.Cache = nil
		return true
	}
	return false
}

type readCap struct {
	n    uint64
	on   bool
	kind selection.SkipKind
}

func contentBudget(size uint64, opts Options) readCap {
	if opts.MaxFileBytesZero {
		return readCap{on: true, kind: selection.SkipOversized}
	}
	if size > 0 {
		if opts.MaxFileBytes > 0 && size > opts.MaxFileBytes {
			return readCap{n: opts.MaxFileBytes, on: true, kind: selection.SkipOversized}
		}
		return readCap{n: size, on: true, kind: selection.SkipConcurrentModification}
	}
	if opts.MaxFileBytes > 0 {
		return readCap{n: opts.MaxFileBytes, on: true, kind: selection.SkipOversized}
	}
	return readCap{}
}

func readBudgeted(r io.Reader, buf []byte, read uint64, capn readCap, rel string) ([]byte, *Skipped, bool, error) {
	if capn.on && read >= capn.n {
		return peekOver(r, capn, rel)
	}
	toRead := len(buf)
	if capn.on && capn.n-read < uint64(toRead) {
		toRead = int(capn.n - read)
	}
	n, err := r.Read(buf[:toRead])
	var chunk []byte
	if n > 0 {
		chunk = buf[:n]
	}
	if err == io.EOF {
		return chunk, nil, true, nil
	}
	if err != nil {
		return nil, &Skipped{Relative: rel, Kind: selection.SkipIOError, Detail: err.Error()}, true, nil
	}
	return chunk, nil, false, nil
}

func peekOver(r io.Reader, capn readCap, rel string) ([]byte, *Skipped, bool, error) {
	var one [1]byte
	n, err := r.Read(one[:])
	if n > 0 {
		return nil, &Skipped{Relative: rel, Kind: capn.kind}, true, nil
	}
	if err == io.EOF || err == nil {
		return nil, nil, true, nil
	}
	return nil, &Skipped{Relative: rel, Kind: selection.SkipIOError, Detail: err.Error()}, true, nil
}

func hashReader(ctx context.Context, r io.Reader, c candidate, opts Options, file ScannedFile, keep bool) (ScannedFile, []byte, *Skipped, CacheStats, error) {
	st := newHashState(opts)
	for {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return file, st.owned, nil, CacheStats{ContentReads: 1}, err
			}
		}
		if scanTimedOut(opts) {
			return file, st.owned, nil, CacheStats{ContentReads: 1}, context.DeadlineExceeded
		}
		chunk, skip, done, err := readBudgeted(r, st.buf, st.read, contentBudget(c.size, opts), c.rel)
		if skip != nil || err != nil {
			return file, st.owned, skip, CacheStats{ContentReads: 1}, err
		}
		if skip := st.add(chunk, opts, c.rel, keep); skip != nil {
			return file, nil, skip, CacheStats{ContentReads: 1}, nil
		}
		if done || st.earlyStop(opts, keep) {
			break
		}
	}
	if opts.ContentValidation == ContentStrict && c.size > 0 && st.read < c.size {
		return file, st.owned, &Skipped{Relative: c.rel, Kind: selection.SkipConcurrentModification}, CacheStats{ContentReads: 1}, nil
	}
	return st.finish(file, opts, keep)
}

type hashState struct {
	h          hash.Hash
	fp         hashx.ContentFingerprint
	needFP     bool
	buf, owned []byte
	read       uint64
}

func newHashState(opts Options) *hashState {
	st := &hashState{buf: make([]byte, 64*1024), needFP: opts.HashFileContents || opts.CacheValidation == CacheStrict, fp: hashx.NewContentFingerprint()}
	if opts.HashFileContents {
		st.h = sha256.New()
	}
	return st
}

func (st *hashState) add(chunk []byte, opts Options, rel string, keep bool) *Skipped {
	if len(chunk) == 0 {
		return nil
	}
	if opts.DetectBinary && bytes.IndexByte(chunk, 0) >= 0 {
		return &Skipped{Relative: rel, Kind: selection.SkipBinary}
	}
	if st.h != nil {
		_, _ = st.h.Write(chunk)
	}
	if st.needFP {
		st.fp.Write(chunk)
	}
	if keep {
		st.owned = append(st.owned, chunk...)
	}
	st.read += uint64(len(chunk))
	return nil
}

func (st *hashState) earlyStop(opts Options, keep bool) bool {
	return !keep && !opts.HashFileContents && st.read >= 8*1024
}

func (st *hashState) finish(file ScannedFile, opts Options, keep bool) (ScannedFile, []byte, *Skipped, CacheStats, error) {
	if st.h != nil {
		file.ContentHash = hashx.Finish(st.h)
	}
	if st.needFP {
		file.ContentFingerprint = st.fp.Finish()
	}
	file.BinaryChecked = opts.DetectBinary
	if keep && st.owned == nil {
		st.owned = []byte{}
	}
	return file, st.owned, nil, CacheStats{ContentReads: 1}, nil
}

func FullFS(ctx context.Context, fsys fs.FS, root string, opts Options) (*Report, error) {
	opts.ensureStarted()
	discovered, err := discoverFS(ctx, fsys, root, opts, true)
	if err != nil {
		return nil, err
	}
	opts.Root, opts.Cache = "", nil
	files, extraSkip, stats, err := inspectFS(ctx, fsys, discovered.candidates, opts)
	if err != nil {
		return nil, err
	}
	report := &Report{
		Root: discovered.root, Files: files, Skipped: append(discovered.skipped, extraSkip...),
		Warnings: discovered.warnings, IgnoreSources: discovered.sources, Complete: discovered.complete,
		Termination: discovered.term, Portable: true, Cache: stats,
	}
	finalize(report, opts)
	return report, nil
}

func PathsFS(ctx context.Context, fsys fs.FS, root string, opts Options) ([]string, error) {
	opts.ensureStarted()
	discovered, err := discoverFS(ctx, fsys, root, pathOpts(opts), false)
	if err != nil {
		return nil, err
	}
	sort.Strings(discovered.paths)
	return discovered.paths, pathTermErr(ctx, discovered)
}
