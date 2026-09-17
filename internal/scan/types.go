package scan

import (
	"context"
	"path/filepath"
	"sort"
	"time"

	"github.com/sergii-ziborov/treestamp/internal/filetypes"
	"github.com/sergii-ziborov/treestamp/internal/ignore"
	"github.com/sergii-ziborov/treestamp/internal/runtime"
	"github.com/sergii-ziborov/treestamp/internal/selection"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

const DescriptorVersion uint32 = 2
const CacheFormatVersion uint32 = 2

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
	MaxEntries    *uint64
	MaxTotalBytes *uint64
	Timeout       time.Duration
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
	discovered, err := discover(ctx, root, pathOpts(opts), false)
	if err != nil {
		return nil, err
	}
	if err := pathTermErr(ctx, discovered); err != nil {
		return nil, err
	}
	sort.Strings(discovered.paths)
	return discovered.paths, nil
}

// StreamPaths emits selected relatives during discovery instead of buffering them.
func StreamPaths(ctx context.Context, root string, opts Options, emit func(string) error) error {
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

func pathOpts(opts Options) Options {
	sel := opts
	sel.HashFileContents, sel.DetectBinary, sel.RecordSkipped = false, false, false
	sel.Walk.CollectMetadata = false
	sel.Cache = nil
	return sel
}

func pathTermErr(ctx context.Context, discovered *discovery) error {
	if discovered == nil {
		return nil
	}
	if discovered.term == TermCancelled {
		if err := ctx.Err(); err != nil {
			return err
		}
		return context.Canceled
	}
	if discovered.term == TermTimeout {
		if err := ctx.Err(); err != nil {
			return err
		}
		return context.DeadlineExceeded
	}
	return nil
}

func Full(ctx context.Context, root string, opts Options) (*Report, error) {
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
