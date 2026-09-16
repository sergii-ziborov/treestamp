package treestamp

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/sergii-ziborov/treestamp/internal/ignore"
	pathx "github.com/sergii-ziborov/treestamp/internal/path"
	"github.com/sergii-ziborov/treestamp/internal/scan"
	"github.com/sergii-ziborov/treestamp/internal/selection"
)

type ScanReport struct {
	Root          string                 `json:"root"`
	Files         []ScannedFile          `json:"files"`
	Skipped       []SkippedEntry         `json:"skipped"`
	Warnings      []ScanWarning          `json:"warnings"`
	IgnoreSources []IgnoreSourceEvidence `json:"ignore_sources"`
	Revision      string                 `json:"revision"`
	Descriptor    ScanDescriptor         `json:"descriptor"`
	Complete      bool                   `json:"complete"`
	Termination   ScanTermination        `json:"termination,omitempty"`
	Portable      bool                   `json:"portable"`
	Cache         ScanCacheStats         `json:"cache"`
}

type CompactScanReport struct {
	Root          string                 `json:"root"`
	Files         []CompactScannedFile   `json:"files"`
	Skipped       []SkippedEntry         `json:"skipped"`
	Warnings      []ScanWarning          `json:"warnings"`
	IgnoreSources []IgnoreSourceEvidence `json:"ignore_sources"`
	Revision      string                 `json:"revision"`
	Descriptor    ScanDescriptor         `json:"descriptor"`
	Complete      bool                   `json:"complete"`
	Termination   ScanTermination        `json:"termination,omitempty"`
	Portable      bool                   `json:"portable"`
	Cache         ScanCacheStats         `json:"cache"`
}

type ScannedFile struct {
	Absolute           string      `json:"absolute"`
	Relative           string      `json:"relative"`
	ContentHash        string      `json:"content_hash,omitempty"`
	ContentFingerprint string      `json:"content_fingerprint,omitempty"`
	Bytes              uint64      `json:"bytes"`
	Version            FileVersion `json:"version"`
	BinaryChecked      bool        `json:"binary_checked"`
	Binary             bool        `json:"binary,omitempty"`
}

type CompactScannedFile struct {
	Relative    string                  `json:"relative"`
	Bytes       uint64                  `json:"bytes"`
	ContentHash string                  `json:"content_hash,omitempty"`
	Content     *CompactContentEvidence `json:"content,omitempty"`
}

type CompactContentEvidence struct {
	ContentHash        string      `json:"content_hash,omitempty"`
	ContentFingerprint string      `json:"content_fingerprint,omitempty"`
	Version            FileVersion `json:"version"`
	BinaryChecked      bool        `json:"binary_checked"`
}

type IgnoreSourceEvidence struct {
	Kind        IgnoreSourceKind `json:"kind"`
	Location    string           `json:"location"`
	ContentHash string           `json:"content_hash"`
}

type ScanCacheStats struct {
	ReusedHashes     uint64 `json:"reused_hashes"`
	ContentReads     uint64 `json:"content_reads"`
	FingerprintReads uint64 `json:"fingerprint_reads"`
	Rebuilt          bool   `json:"rebuilt,omitempty"`
}

type SkippedEntry struct {
	Relative string   `json:"relative"`
	Kind     SkipKind `json:"kind"`
	Detail   string   `json:"detail,omitempty"`
}

type ScanWarning struct {
	Relative string `json:"relative,omitempty"`
	Message  string `json:"message"`
}

type ScanDescriptor struct {
	Version uint32 `json:"version"`
	Policy  string `json:"policy"`
}

func (d ScanDescriptor) Matches(opts Options) bool {
	return scan.Descriptor{Version: d.Version, Policy: d.Policy}.Matches(toScanOptions(opts))
}

type SkipKind = selection.SkipKind

const (
	SkipNone                   = selection.SkipNone
	SkipBinary                 = selection.SkipBinary
	SkipFileSystemBoundary     = selection.SkipFileSystemBoundary
	SkipExtension              = selection.SkipExtension
	SkipIgnored                = selection.SkipIgnored
	SkipIOError                = selection.SkipIOError
	SkipMaxDepth               = selection.SkipMaxDepth
	SkipOversized              = selection.SkipOversized
	SkipPathEscape             = selection.SkipPathEscape
	SkipStandardDirectory      = selection.SkipStandardDirectory
	SkipHidden                 = selection.SkipHidden
	SkipOverride               = selection.SkipOverride
	SkipSymlink                = selection.SkipSymlink
	SkipSymlinkLoop            = selection.SkipSymlinkLoop
	SkipScanLimit              = selection.SkipScanLimit
	SkipConcurrentModification = selection.SkipConcurrentModification
)

type Scanner struct {
	root    string
	options Options
}

func Scan(ctx context.Context, root string) (*ScanReport, error) {
	s, err := NewScanner(root)
	if err != nil {
		return nil, err
	}
	return s.Scan(ctx)
}
func ScanCompact(ctx context.Context, root string) (*CompactScanReport, error) {
	s, err := NewScanner(root)
	if err != nil {
		return nil, err
	}
	return s.ScanCompact(ctx)
}
func ScanPaths(ctx context.Context, root string) ([]string, error) {
	s, err := NewScanner(root)
	if err != nil {
		return nil, err
	}
	return s.ScanPaths(ctx)
}

func NewScanner(root string, opts ...ScannerOption) (*Scanner, error) {
	if root == "" {
		return nil, &Error{Code: CodeInvalid, Op: "NewScanner", Err: errEmptyRoot}
	}
	cfg := scannerConfig{options: DefaultOptions()}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.traversalWorkers > 0 {
		cfg.options.TraversalWorkers = cfg.traversalWorkers
	}
	if cfg.contentWorkers > 0 {
		cfg.options.ContentWorkers = cfg.contentWorkers
	}
	if err := cfg.options.Filters.Err(); err != nil {
		return nil, &Error{Code: CodeInvalid, Op: "NewScanner", Err: err}
	}
	return &Scanner{root: root, options: cfg.options}, nil
}

func (s *Scanner) Options(options Options) *Scanner { s.options = options; return s }

func (s *Scanner) runFull(ctx context.Context, op string, fn func() (*scan.Report, error)) (*ScanReport, error) {
	if s == nil {
		return nil, &Error{Code: CodeInvalid, Op: op, Err: errEmptyRoot}
	}
	if err := s.options.Filters.Err(); err != nil {
		return nil, &Error{Code: CodeInvalid, Op: op, Err: err}
	}
	report, err := fn()
	if err != nil {
		return nil, wrap(err, op, s.root)
	}
	return fromFull(report), nil
}
func (s *Scanner) Scan(ctx context.Context) (*ScanReport, error) {
	return s.runFull(ctx, "Scan", func() (*scan.Report, error) { return scan.Full(ctx, s.root, toScanOptions(s.options)) })
}
func (s *Scanner) ScanCompact(ctx context.Context) (*CompactScanReport, error) {
	report, err := scan.Compact(ctx, s.root, toScanOptions(s.options))
	if err != nil {
		return nil, wrap(err, "ScanCompact", s.root)
	}
	return fromCompact(report), nil
}
func (s *Scanner) ScanPaths(ctx context.Context) ([]string, error) {
	if err := s.options.Filters.Err(); err != nil {
		return nil, &Error{Code: CodeInvalid, Op: "ScanPaths", Err: err}
	}
	paths, err := scan.Paths(ctx, s.root, toScanOptions(s.options))
	if err != nil {
		return nil, wrap(err, "ScanPaths", s.root)
	}
	return paths, nil
}
func (s *Scanner) ScanIncremental(ctx context.Context, previous *ScanReport) (*ScanReport, error) {
	return s.runFull(ctx, "ScanIncremental", func() (*scan.Report, error) {
		return scan.Incremental(ctx, s.root, toScanOptions(s.options), toInternalReport(previous))
	})
}
func (s *Scanner) ScanCached(ctx context.Context, cache *ScanCache) (*ScanReport, error) {
	var inner *scan.Cache
	if cache != nil {
		c := toInternalCache(cache)
		inner = &c
	}
	return s.runFull(ctx, "ScanCached", func() (*scan.Report, error) {
		return scan.Cached(ctx, s.root, toScanOptions(s.options), inner)
	})
}
func (s *Scanner) ScanWatchPlan(ctx context.Context, previous *ScanReport, plan WatchPlan) (*ScanReport, error) {
	update, err := s.ScanWatchPlanDetailed(ctx, previous, plan)
	if err != nil {
		return nil, err
	}
	return update.Report, nil
}
func (s *Scanner) ScanWatchPlanDetailed(ctx context.Context, previous *ScanReport, plan WatchPlan) (*WatchUpdate, error) {
	update, err := scan.Watch(ctx, s.root, toScanOptions(s.options), toInternalReport(previous), toInternalPlan(plan))
	if err != nil {
		return nil, wrap(err, "ScanWatchPlan", s.root)
	}
	return &WatchUpdate{Report: fromFull(update.Report), Reason: watchReason(update.Reason)}, nil
}

func wrap(err error, op, path string) error {
	if err == nil {
		return nil
	}
	if typed, ok := err.(*Error); ok {
		return typed
	}
	return &Error{Code: classifyCode(err), Op: op, Path: path, Err: err}
}

func copySkipped(in []scan.Skipped) []SkippedEntry {
	out := make([]SkippedEntry, len(in))
	for i, s := range in {
		out[i] = SkippedEntry{Relative: s.Relative, Kind: s.Kind, Detail: s.Detail}
	}
	return out
}
func copyWarnings(in []scan.Warning) []ScanWarning {
	out := make([]ScanWarning, len(in))
	for i, w := range in {
		out[i] = ScanWarning{Relative: w.Relative, Message: w.Message}
	}
	return out
}
func copySources(in []scan.IgnoreSource) []IgnoreSourceEvidence {
	out := make([]IgnoreSourceEvidence, len(in))
	for i, src := range in {
		out[i] = IgnoreSourceEvidence{Kind: src.Kind, Location: src.Location, ContentHash: src.ContentHash}
	}
	return out
}

func descOf(version uint32, policy string) ScanDescriptor {
	return ScanDescriptor{Version: version, Policy: policy}
}

func fromFull(report *scan.Report) *ScanReport {
	out := &ScanReport{
		Root: report.Root, Revision: report.Revision, Descriptor: descOf(report.Descriptor.Version, report.Descriptor.Policy),
		Complete: report.Complete, Termination: ScanTermination(report.Termination), Portable: report.Portable,
		Cache: ScanCacheStats(report.Cache), Files: make([]ScannedFile, len(report.Files)),
		Skipped: copySkipped(report.Skipped), Warnings: copyWarnings(report.Warnings), IgnoreSources: copySources(report.IgnoreSources),
	}
	for i, f := range report.Files {
		out.Files[i] = publicFile(f)
	}
	return out
}

func publicFile(f scan.ScannedFile) ScannedFile {
	return ScannedFile{Absolute: f.Absolute, Relative: f.Relative, Bytes: f.Bytes, ContentHash: f.ContentHash, ContentFingerprint: f.ContentFingerprint, Version: f.Version, BinaryChecked: f.BinaryChecked}
}

func fromCompact(report *scan.CompactReport) *CompactScanReport {
	out := &CompactScanReport{
		Root: report.Root, Revision: report.Revision, Descriptor: descOf(report.Descriptor.Version, report.Descriptor.Policy),
		Complete: report.Complete, Termination: ScanTermination(report.Termination), Portable: report.Portable,
		Cache: ScanCacheStats(report.Cache), Files: make([]CompactScannedFile, len(report.Files)),
		Skipped: copySkipped(report.Skipped), Warnings: copyWarnings(report.Warnings), IgnoreSources: copySources(report.IgnoreSources),
	}
	for i, f := range report.Files {
		item := CompactScannedFile{Relative: f.Relative, Bytes: f.Bytes, ContentHash: f.ContentHash()}
		if f.Content != nil {
			item.Content = &CompactContentEvidence{ContentHash: f.Content.ContentHash, ContentFingerprint: f.Content.ContentFingerprint, Version: f.Content.Version, BinaryChecked: f.Content.BinaryChecked}
		}
		out.Files[i] = item
	}
	return out
}

func toInternalReport(report *ScanReport) *scan.Report {
	if report == nil {
		return nil
	}
	out := &scan.Report{
		Root: report.Root, Revision: report.Revision,
		Descriptor: scan.Descriptor{Version: report.Descriptor.Version, Policy: report.Descriptor.Policy},
		Complete:   report.Complete, Termination: scan.Termination(report.Termination), Portable: report.Portable,
		Cache: scan.CacheStats(report.Cache),
	}
	for _, f := range report.Files {
		out.Files = append(out.Files, scan.ScannedFile{Absolute: f.Absolute, Relative: f.Relative, Bytes: f.Bytes, ContentHash: f.ContentHash, ContentFingerprint: f.ContentFingerprint, Version: f.Version, BinaryChecked: f.BinaryChecked})
	}
	out.Skipped = copyInternalSkipped(report.Skipped)
	out.Warnings = copyInternalWarnings(report.Warnings)
	out.IgnoreSources = copyInternalSources(report.IgnoreSources)
	return out
}

func copyInternalSkipped(in []SkippedEntry) []scan.Skipped {
	out := make([]scan.Skipped, len(in))
	for i, s := range in {
		out[i] = scan.Skipped{Relative: s.Relative, Kind: s.Kind, Detail: s.Detail}
	}
	return out
}
func copyInternalWarnings(in []ScanWarning) []scan.Warning {
	out := make([]scan.Warning, len(in))
	for i, w := range in {
		out[i] = scan.Warning{Relative: w.Relative, Message: w.Message}
	}
	return out
}
func copyInternalSources(in []IgnoreSourceEvidence) []scan.IgnoreSource {
	out := make([]scan.IgnoreSource, len(in))
	for i, src := range in {
		out[i] = scan.IgnoreSource{Kind: src.Kind, Location: src.Location, ContentHash: src.ContentHash}
	}
	return out
}

func toInternalCache(cache *ScanCache) scan.Cache {
	out := scan.Cache{FormatVersion: cache.FormatVersion, Root: cache.Root}
	for _, e := range cache.Entries {
		out.Entries = append(out.Entries, scan.CacheEntry{Relative: e.Relative, Bytes: e.Bytes, ContentHash: e.ContentHash, ContentFingerprint: e.ContentFingerprint, Version: e.Version, BinaryChecked: e.BinaryChecked})
	}
	return out
}

func toInternalPlan(plan WatchPlan) scan.WatchPlan {
	return scan.WatchPlan{Changed: plan.Changed, Removed: plan.Removed, FullRescan: plan.FullRescan, RejectedEvents: plan.RejectedEvents}
}

type ScanOptions = Options

func DescriptorFromOptions(opts Options) ScanDescriptor {
	inner := scan.DescriptorFromOptions(toScanOptions(opts))
	return ScanDescriptor{Version: inner.Version, Policy: inner.Policy}
}

type reportMeta struct {
	Root, Revision string
	Descriptor     ScanDescriptor
	Complete       bool
	Termination    ScanTermination
	Portable       bool
	Cache          ScanCacheStats
	Skipped        []SkippedEntry
	Warnings       []ScanWarning
	IgnoreSources  []IgnoreSourceEvidence
}

func cloneMeta(m reportMeta) reportMeta {
	m.Skipped = append([]SkippedEntry(nil), m.Skipped...)
	m.Warnings = append([]ScanWarning(nil), m.Warnings...)
	m.IgnoreSources = append([]IgnoreSourceEvidence(nil), m.IgnoreSources...)
	return m
}

func (r *ScanReport) ToCompact() CompactScanReport {
	if r == nil {
		return CompactScanReport{}
	}
	out := CompactScanReport{}
	m := cloneMeta(r.meta())
	out.Root, out.Revision, out.Descriptor, out.Complete, out.Termination, out.Portable, out.Cache, out.Skipped, out.Warnings, out.IgnoreSources = m.Root, m.Revision, m.Descriptor, m.Complete, m.Termination, m.Portable, m.Cache, m.Skipped, m.Warnings, m.IgnoreSources
	out.Files = make([]CompactScannedFile, len(r.Files))
	for i, file := range r.Files {
		item := CompactScannedFile{Relative: file.Relative, Bytes: file.Bytes, ContentHash: file.ContentHash}
		if file.ContentHash != "" || file.ContentFingerprint != "" || file.BinaryChecked {
			item.Content = &CompactContentEvidence{ContentHash: file.ContentHash, ContentFingerprint: file.ContentFingerprint, Version: file.Version, BinaryChecked: file.BinaryChecked}
		}
		out.Files[i] = item
	}
	return out
}

func (r CompactScanReport) AbsolutePath(file CompactScannedFile) string {
	return filepath.Join(r.Root, filepath.FromSlash(file.Relative))
}

func (r CompactScanReport) IntoScanReport() ScanReport {
	out := ScanReport{}
	m := cloneMeta(r.meta())
	out.Root, out.Revision, out.Descriptor, out.Complete, out.Termination, out.Portable, out.Cache, out.Skipped, out.Warnings, out.IgnoreSources = m.Root, m.Revision, m.Descriptor, m.Complete, m.Termination, m.Portable, m.Cache, m.Skipped, m.Warnings, m.IgnoreSources
	out.Files = make([]ScannedFile, len(r.Files))
	for i, file := range r.Files {
		item := ScannedFile{Absolute: r.AbsolutePath(file), Relative: file.Relative, Bytes: file.Bytes}
		if file.Content != nil {
			item.ContentHash, item.ContentFingerprint, item.Version, item.BinaryChecked = file.Content.ContentHash, file.Content.ContentFingerprint, file.Content.Version, file.Content.BinaryChecked
		} else {
			item.ContentHash = file.ContentHash
		}
		out.Files[i] = item
	}
	return out
}

func (f ScannedFile) AbsolutePath() string                { return f.Absolute }
func (r *ScanReport) Delta(current *ScanReport) ScanDelta { return DeltaBetween(r, current) }

type ScanCacheEntry struct {
	Relative           string      `json:"relative"`
	ContentHash        string      `json:"content_hash,omitempty"`
	ContentFingerprint string      `json:"content_fingerprint,omitempty"`
	Bytes              uint64      `json:"bytes"`
	Version            FileVersion `json:"version"`
	BinaryChecked      bool        `json:"binary_checked"`
}

type ScanCache struct {
	FormatVersion uint32           `json:"format_version"`
	Root          string           `json:"root"`
	Entries       []ScanCacheEntry `json:"entries"`
}

func (r *ScanReport) ToCache() ScanCache {
	if r == nil {
		return ScanCache{FormatVersion: ScanCacheFormatVersion}
	}
	return fromInternalCache(scan.CacheFromReport(toInternalReport(r)))
}
func (r *CompactScanReport) ToCache() ScanCache {
	if r == nil {
		return ScanCache{FormatVersion: ScanCacheFormatVersion}
	}
	return fromInternalCache(scan.CacheFromCompact(toInternalCompact(r)))
}
func (c ScanCache) IsCompatible(root string) bool {
	return c.FormatVersion == ScanCacheFormatVersion && c.Root == root
}
func (c *ScanCache) Invalidate(relativePaths []string) int {
	if c == nil {
		return 0
	}
	prefixes := pathx.CollapsePathPrefixes(relativePaths)
	before := len(c.Entries)
	keep := c.Entries[:0]
	for _, entry := range c.Entries {
		if !pathx.PathCoveredByPrefixes(entry.Relative, prefixes) {
			keep = append(keep, entry)
		}
	}
	c.Entries = keep
	return before - len(c.Entries)
}
func (c *ScanCache) ApplyWatchPlan(plan WatchPlan) int {
	if plan.FullRescan {
		n := len(c.Entries)
		c.Entries = nil
		return n
	}
	return c.Invalidate(plan.Invalidated())
}

func fromInternalCache(c scan.Cache) ScanCache {
	out := ScanCache{FormatVersion: c.FormatVersion, Root: c.Root}
	for _, e := range c.Entries {
		out.Entries = append(out.Entries, ScanCacheEntry{Relative: e.Relative, Bytes: e.Bytes, ContentHash: e.ContentHash, ContentFingerprint: e.ContentFingerprint, Version: e.Version, BinaryChecked: e.BinaryChecked})
	}
	return out
}

func toInternalCompact(r *CompactScanReport) *scan.CompactReport {
	out := &scan.CompactReport{
		Root: r.Root, Revision: r.Revision,
		Descriptor: scan.Descriptor{Version: r.Descriptor.Version, Policy: r.Descriptor.Policy},
		Complete:   r.Complete, Termination: scan.Termination(r.Termination), Portable: r.Portable,
		Cache: scan.CacheStats(r.Cache),
	}
	for _, f := range r.Files {
		item := scan.CompactFile{Relative: f.Relative, Bytes: f.Bytes}
		if f.Content != nil {
			item.Content = &scan.ContentEvidence{ContentHash: f.Content.ContentHash, ContentFingerprint: f.Content.ContentFingerprint, Version: f.Content.Version, BinaryChecked: f.Content.BinaryChecked}
		}
		out.Files = append(out.Files, item)
	}
	return out
}

type ScanSummary struct {
	SelectedFiles, HashedFiles, BinaryCheckedFiles, RecordedSkips, Warnings, IgnoreSources int
	SelectedBytes                                                                          uint64
	SkippedByKind                                                                          map[SkipKind]int
	Complete, Stopped                                                                      bool
	Termination                                                                            ScanTermination
	Portable                                                                               bool
	Cache                                                                                  ScanCacheStats
}

func fillSummary(m reportMeta, selected int) ScanSummary {
	sum := ScanSummary{
		SelectedFiles: selected, RecordedSkips: len(m.Skipped), Warnings: len(m.Warnings), IgnoreSources: len(m.IgnoreSources),
		Complete: m.Complete, Termination: m.Termination, Portable: m.Portable, Cache: m.Cache, SkippedByKind: map[SkipKind]int{},
	}
	for _, skipped := range m.Skipped {
		sum.SkippedByKind[skipped.Kind]++
	}
	return sum
}

func (r *ScanReport) meta() reportMeta {
	return reportMeta{Root: r.Root, Revision: r.Revision, Descriptor: r.Descriptor, Complete: r.Complete, Termination: r.Termination, Portable: r.Portable, Cache: r.Cache, Skipped: r.Skipped, Warnings: r.Warnings, IgnoreSources: r.IgnoreSources}
}

func (r *CompactScanReport) meta() reportMeta {
	return reportMeta{Root: r.Root, Revision: r.Revision, Descriptor: r.Descriptor, Complete: r.Complete, Termination: r.Termination, Portable: r.Portable, Cache: r.Cache, Skipped: r.Skipped, Warnings: r.Warnings, IgnoreSources: r.IgnoreSources}
}

func (r *ScanReport) Summary() ScanSummary {
	sum := fillSummary(r.meta(), len(r.Files))
	for _, file := range r.Files {
		sum.SelectedBytes += file.Bytes
		if file.ContentHash != "" {
			sum.HashedFiles++
		}
		if file.BinaryChecked {
			sum.BinaryCheckedFiles++
		}
	}
	return sum
}

func (r *CompactScanReport) Summary() ScanSummary {
	sum := fillSummary(r.meta(), len(r.Files))
	for _, file := range r.Files {
		sum.SelectedBytes += file.Bytes
		if file.ContentHash != "" || (file.Content != nil && file.Content.ContentHash != "") {
			sum.HashedFiles++
		}
		if file.Content != nil && file.Content.BinaryChecked {
			sum.BinaryCheckedFiles++
		}
	}
	return sum
}

func (s ScanSummary) String() string {
	out := fmt.Sprintf("files=%d bytes=%d hashed=%d binary_checked=%d skipped=%d warnings=%d ignore_sources=%d complete=%t portable=%t",
		s.SelectedFiles, s.SelectedBytes, s.HashedFiles, s.BinaryCheckedFiles, s.RecordedSkips, s.Warnings, s.IgnoreSources, s.Complete, s.Portable)
	if s.Termination != TerminationNone {
		out += " termination=" + s.Termination.String()
	}
	if s.Cache != (ScanCacheStats{}) {
		out += fmt.Sprintf(" cache_reused_hashes=%d cache_content_reads=%d cache_fingerprint_reads=%d", s.Cache.ReusedHashes, s.Cache.ContentReads, s.Cache.FingerprintReads)
	}
	return out
}

type RepositoryMatcher struct {
	engine *ignore.Engine
	root   string
}

func NewRepositoryMatcher(root string, opts Options) (*RepositoryMatcher, error) {
	eng := ignore.NewEngine(opts.IgnoreFiles, opts.IgnoreCase, opts.OverrideRules)
	eng.SetGitModules(opts.GitModules)
	if opts.IgnorePolicy.inner.Specified() {
		eng.ApplyPolicy(root, opts.IgnorePolicy.inner)
	} else {
		eng.ApplyPolicy(root, ignore.RepositoryPolicy())
	}
	eng.LoadDir(root, "")
	return &RepositoryMatcher{engine: eng, root: root}, nil
}

var errEmptyRoot = errString("root is empty")

type errString string

func (e errString) Error() string { return string(e) }
