package treestamp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sergii-ziborov/treestamp/internal/filetypes"
	"github.com/sergii-ziborov/treestamp/internal/ignore"
	"github.com/sergii-ziborov/treestamp/internal/report"
	"github.com/sergii-ziborov/treestamp/internal/runtime"
	"github.com/sergii-ziborov/treestamp/internal/scan"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

type Options struct {
	MaxFileBytes      uint64
	IgnoreFiles       []string
	OverrideRules     []string
	Extensions        []string
	FileTypes         *filetypes.NamedFileTypes
	IgnorePolicy      IgnorePolicy
	IgnoreCase        bool
	SkipHidden        bool
	StandardSkips     bool
	HashFileContents  bool
	DetectBinaryFiles bool
	EvidenceComplete  bool
	Evidence          EvidenceMode
	Walk              WalkOptions
	TraversalWorkers  int
	ContentWorkers    int
	Limits            ScanLimits
	Cancellation      *CancellationToken
	CacheValidation   CacheValidationPolicy
	ContentValidation ContentValidationPolicy
	ContentDiscovery  ContentDiscoveryMode
}

type NamedFileTypes = filetypes.NamedFileTypes

func DefaultFileTypes() *NamedFileTypes { return filetypes.Defaults() }

func DefaultOptions() Options {
	w := walk.DefaultOptions()
	w.CollectMetadata = true
	return Options{
		MaxFileBytes: 1_500_000, IgnoreFiles: []string{".gitignore", ".ignore", ".weavatrixignore"},
		IgnorePolicy: RepositoryIgnorePolicy(), StandardSkips: true, HashFileContents: true,
		DetectBinaryFiles: true, EvidenceComplete: true, Evidence: EvidenceComplete, Walk: w,
		CacheValidation: CacheValidationFast, ContentValidation: ContentValidationStrict,
		ContentDiscovery: ContentDiscoveryStreaming,
	}
}

func toScanOptions(opts Options) scan.Options {
	w := opts.Walk
	if w.MaxOpen == 0 {
		w = walk.DefaultOptions()
		w.CollectMetadata = true
	}
	record := opts.EvidenceComplete
	if opts.Evidence == EvidenceSelectedFiles {
		record = false
	}
	out := scan.Options{
		MaxFileBytes: opts.MaxFileBytes, IgnoreFiles: opts.IgnoreFiles, OverrideRules: opts.OverrideRules,
		Extensions: opts.Extensions, FileTypes: opts.FileTypes, IgnorePolicy: opts.IgnorePolicy.inner,
		IgnoreCase: opts.IgnoreCase, SkipHidden: opts.SkipHidden, StandardSkips: opts.StandardSkips,
		HashFileContents: opts.HashFileContents, DetectBinary: opts.DetectBinaryFiles, RecordSkipped: record,
		Walk: w, TraversalWorkers: opts.TraversalWorkers, ContentWorkers: opts.ContentWorkers,
		Limits:          scan.Limits{MaxEntries: opts.Limits.MaxEntries, MaxTotalBytes: opts.Limits.MaxTotalBytes, Timeout: opts.Limits.Timeout},
		CacheValidation: scan.CacheValidation(opts.CacheValidation), ContentValidation: scan.ContentValidation(opts.ContentValidation),
		ContentDiscovery: scan.ContentDiscovery(opts.ContentDiscovery),
	}
	if !out.IgnorePolicy.Specified() {
		out.IgnorePolicy = ignore.RepositoryPolicy()
	}
	if opts.Cancellation != nil {
		out.Cancel = &opts.Cancellation.inner
	}
	return out
}

type ScannerOption func(*scannerConfig)
type scannerConfig struct {
	options          Options
	traversalWorkers int
	contentWorkers   int
}

func WithOptions(options Options) ScannerOption {
	return func(cfg *scannerConfig) { cfg.options = options }
}
func WithTraversalWorkers(n int) ScannerOption {
	return func(cfg *scannerConfig) { cfg.traversalWorkers = n }
}
func WithContentWorkers(n int) ScannerOption {
	return func(cfg *scannerConfig) { cfg.contentWorkers = n }
}

func (o Options) WithExtensions(exts ...string) Options {
	cleaned := make([]string, 0, len(exts))
	for _, ext := range exts {
		cleaned = append(cleaned, strings.TrimPrefix(strings.ToLower(ext), "."))
	}
	o.Extensions = cleaned
	return o
}
func (o Options) WithFileTypes(types *NamedFileTypes) Options { o.FileTypes = types; return o }
func (o Options) WithIgnoreFiles(names ...string) Options {
	o.IgnoreFiles = append([]string(nil), names...)
	return o
}
func (o Options) WithOverrideRules(rules ...string) Options {
	o.OverrideRules = append([]string(nil), rules...)
	return o
}
func (o Options) WithIgnoreCase(enabled bool) Options          { o.IgnoreCase = enabled; return o }
func (o Options) WithIgnorePolicy(policy IgnorePolicy) Options { o.IgnorePolicy = policy; return o }
func (o Options) WithSkipHidden(enabled bool) Options          { o.SkipHidden = enabled; return o }
func (o Options) WithStandardSkips(enabled bool) Options       { o.StandardSkips = enabled; return o }
func (o Options) MetadataOnly() Options {
	o.HashFileContents, o.DetectBinaryFiles = false, false
	return o
}
func (o Options) SelectedFilesOnly() Options {
	o.Evidence, o.EvidenceComplete = EvidenceSelectedFiles, false
	return o
}
func (o Options) WithParallelism(n int) Options {
	o.TraversalWorkers, o.ContentWorkers = n, n
	return o
}
func (o Options) WithTraversalParallelism(n int) Options { o.TraversalWorkers = n; return o }
func (o Options) WithContentParallelism(n int) Options   { o.ContentWorkers = n; return o }
func (o Options) WithContentDiscovery(mode ContentDiscoveryMode) Options {
	o.ContentDiscovery = mode
	return o
}
func (o Options) WithMaxEntries(n uint64) Options                   { o.Limits.MaxEntries = &n; return o }
func (o Options) WithMaxTotalBytes(n uint64) Options                { o.Limits.MaxTotalBytes = &n; return o }
func (o Options) WithTimeout(d time.Duration) Options               { o.Limits.Timeout = d; return o }
func (o Options) WithCancellation(token *CancellationToken) Options { o.Cancellation = token; return o }
func (o Options) WithCacheValidation(policy CacheValidationPolicy) Options {
	o.CacheValidation = policy
	return o
}
func (o Options) WithContentValidation(policy ContentValidationPolicy) Options {
	o.ContentValidation = policy
	return o
}
func (o Options) WithMaxFileBytes(n uint64) Options { o.MaxFileBytes = n; return o }

const (
	ScanDescriptorVersion  = scan.DescriptorVersion
	ScanCacheFormatVersion = scan.CacheFormatVersion
)

type StandardSkips int

const (
	StandardSkipsEnabled StandardSkips = iota
	StandardSkipsDisabled
)

type EvidenceMode int

const (
	EvidenceComplete EvidenceMode = iota
	EvidenceSelectedFiles
)

type CacheValidationPolicy int

const (
	CacheValidationFast CacheValidationPolicy = iota
	CacheValidationStrict
)

type ContentValidationPolicy int

const (
	ContentValidationFast ContentValidationPolicy = iota
	ContentValidationStrict
)

type ContentDiscoveryMode int

const (
	ContentDiscoveryStreaming ContentDiscoveryMode = iota
	ContentDiscoveryBufferedParallel
)

type ScanLimits struct {
	MaxEntries    *uint64
	MaxTotalBytes *uint64
	Timeout       time.Duration
}

type IgnorePolicy struct{ inner ignore.Policy }

func RepositoryIgnorePolicy() IgnorePolicy {
	return IgnorePolicy{inner: ignore.RepositoryPolicy()}
}
func NoneIgnorePolicy() IgnorePolicy { return IgnorePolicy{inner: ignore.NonePolicy()} }
func GitCompatibleIgnorePolicy() IgnorePolicy {
	return IgnorePolicy{inner: ignore.GitCompatiblePolicy()}
}
func (p IgnorePolicy) WithParentRules(enabled bool) IgnorePolicy {
	p.inner.ParentRules = enabled
	return p
}
func (p IgnorePolicy) WithGitIgnore(enabled bool) IgnorePolicy { p.inner.GitIgnore = enabled; return p }
func (p IgnorePolicy) WithDotIgnore(enabled bool) IgnorePolicy { p.inner.DotIgnore = enabled; return p }
func (p IgnorePolicy) WithCustomIgnore(enabled bool) IgnorePolicy {
	p.inner.CustomIgnore = enabled
	return p
}
func (p IgnorePolicy) WithGitExclude(enabled bool) IgnorePolicy {
	p.inner.GitExclude = enabled
	return p
}
func (p IgnorePolicy) WithGitGlobal(enabled bool) IgnorePolicy { p.inner.GitGlobal = enabled; return p }
func (p IgnorePolicy) WithRequireGit(enabled bool) IgnorePolicy {
	p.inner.RequireGit = enabled
	return p
}
func (p IgnorePolicy) WithExplicitFile(path string) IgnorePolicy {
	p.inner.ExplicitFiles = append(p.inner.ExplicitFiles, path)
	return p
}

type IgnoreSourceKind = ignore.SourceKind

const (
	IgnoreGitGlobal  = ignore.SourceGitGlobal
	IgnoreGitExclude = ignore.SourceGitExclude
	IgnoreGitIgnore  = ignore.SourceGitIgnore
	IgnoreDotIgnore  = ignore.SourceDotIgnore
	IgnoreCustom     = ignore.SourceCustom
	IgnoreExplicit   = ignore.SourceExplicit
	IgnoreOverride   = ignore.SourceOverride
)

type IgnoreFile struct {
	Kind     IgnoreSourceKind
	Location string
}

type CancellationToken struct{ inner runtime.Token }

func NewCancellationToken() *CancellationToken { return &CancellationToken{} }
func (t *CancellationToken) Cancel() {
	if t != nil {
		t.inner.Cancel()
	}
}
func (t *CancellationToken) IsCancelled() bool { return t != nil && t.inner.Cancelled() }

type ScanTermination int

const (
	TerminationNone ScanTermination = iota
	TerminationMaxEntries
	TerminationMaxTotalBytes
	TerminationTimeout
	TerminationCancelled
)

func (t ScanTermination) String() string {
	names := [...]string{"", "max_entries", "max_total_bytes", "timeout", "cancelled"}
	if int(t) >= 0 && int(t) < len(names) {
		return names[t]
	}
	return ""
}

type ErrorCode string

const (
	CodeNotImplemented ErrorCode = "not_implemented"
	CodeInvalid        ErrorCode = "invalid"
	CodeWalk           ErrorCode = "walk"
	CodeCancelled      ErrorCode = "cancelled"
	CodeTimeout        ErrorCode = "timeout"
	CodeStale          ErrorCode = "stale_snapshot"
)

type Error struct {
	Code ErrorCode
	Op   string
	Path string
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Path != "" && e.Err != nil {
		return fmt.Sprintf("treestamp: %s %s: %s: %v", e.Code, e.Op, e.Path, e.Err)
	}
	if e.Path != "" {
		return fmt.Sprintf("treestamp: %s %s: %s", e.Code, e.Op, e.Path)
	}
	if e.Err != nil {
		return fmt.Sprintf("treestamp: %s %s: %v", e.Code, e.Op, e.Err)
	}
	return fmt.Sprintf("treestamp: %s %s", e.Code, e.Op)
}
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
func IsNotImplemented(err error) bool {
	var typed *Error
	return errors.As(err, &typed) && typed.Code == CodeNotImplemented
}

type SnapshotEvidence int

const (
	SnapshotFileVersion SnapshotEvidence = iota
	SnapshotSHA256
)

type SnapshotContent struct {
	Relative string
	Bytes    []byte
	Evidence SnapshotEvidence
}

type SnapshotReadError struct {
	Relative        string
	Reason          string
	Bytes, MaxBytes uint64
	Err             error
}

func (e *SnapshotReadError) Error() string {
	if e == nil {
		return "snapshot read error"
	}
	return (&report.Error{Relative: e.Relative, Reason: e.Reason, Bytes: e.Bytes, MaxBytes: e.MaxBytes, Err: e.Err}).Error()
}
func (e *SnapshotReadError) Unwrap() error { return e.Err }

func mapSnapErr(err error) error {
	var inner *report.Error
	if errors.As(err, &inner) {
		return &SnapshotReadError{Relative: inner.Relative, Reason: inner.Reason, Bytes: inner.Bytes, MaxBytes: inner.MaxBytes, Err: inner.Err}
	}
	return err
}

func toReportFiles(files []ScannedFile) []report.File {
	out := make([]report.File, len(files))
	for i, file := range files {
		out[i] = report.File{Absolute: file.Absolute, Relative: file.Relative, ContentHash: file.ContentHash, Bytes: file.Bytes, ModifiedNS: file.Version.ModifiedNS}
	}
	return out
}

type SnapshotContentProvider struct {
	root  string
	files []ScannedFile
}

func (r *ScanReport) ContentProvider() (*SnapshotContentProvider, error) {
	if r == nil {
		return nil, &SnapshotReadError{Reason: "invalid", Err: errors.New("empty snapshot")}
	}
	if err := validateSnapshotReport(r); err != nil {
		return nil, err
	}
	return &SnapshotContentProvider{root: r.Root, files: r.Files}, nil
}

func (p *SnapshotContentProvider) Open(relative string) (*SnapshotContent, error) {
	return p.Read(relative)
}
func (p *SnapshotContentProvider) Read(relative string) (*SnapshotContent, error) {
	return p.ReadBounded(relative, ^uint64(0))
}
func (p *SnapshotContentProvider) ReadBounded(relative string, maxBytes uint64) (*SnapshotContent, error) {
	if p == nil {
		return nil, &SnapshotReadError{Relative: relative, Reason: "invalid", Err: errors.New("empty snapshot")}
	}
	data, evidence, err := report.ReadBounded(p.root, toReportFiles(p.files), relative, maxBytes)
	if err != nil {
		return nil, mapSnapErr(err)
	}
	return &SnapshotContent{Relative: relative, Bytes: data, Evidence: SnapshotEvidence(evidence)}, nil
}

func validateSnapshotReport(rep *ScanReport) error {
	return mapSnapErr(report.Validate(rep.Root, toReportFiles(rep.Files)))
}

type ScanSession struct {
	path       string
	options    Options
	report     *ScanReport
	generation uint64
	lastReason WatchUpdateReason
	hasReason  bool
}

func scanRoot(ctx context.Context, root string, opts Options) (*ScanReport, error) {
	scanner, err := NewScanner(root, WithOptions(opts))
	if err != nil {
		return nil, err
	}
	return scanner.Scan(ctx)
}

func OpenScanSession(ctx context.Context, root string, opts Options) (*ScanSession, error) {
	report, err := scanRoot(ctx, root, opts)
	if err != nil {
		return nil, err
	}
	opts.Cancellation = nil
	return &ScanSession{path: root, options: opts, report: report, generation: 1}, nil
}

func (s *ScanSession) Root() string            { return s.path }
func (s *ScanSession) Report() *ScanReport     { return s.report }
func (s *ScanSession) Generation() uint64      { return s.generation }
func (s *ScanSession) IntoReport() *ScanReport { return s.report }
func (s *ScanSession) LastUpdateReason() (WatchUpdateReason, bool) {
	return s.lastReason, s.hasReason
}
func (s *ScanSession) FilesPage(generation uint64, offset, limit int) ([]PortableScannedFile, error) {
	if generation != s.generation {
		return nil, &Error{Code: CodeStale, Op: "FilesPage"}
	}
	return s.report.PortableFilesPage(offset, limit), nil
}
func (s *ScanSession) Rescan(ctx context.Context) (*ScanReport, error) {
	report, err := scanRoot(ctx, s.path, s.options)
	if err != nil {
		return nil, err
	}
	s.install(report, WatchReasonFullStructural)
	return s.report, nil
}
func (s *ScanSession) ApplyWatchPlan(ctx context.Context, plan WatchPlan) (*ScanReport, error) {
	return s.ApplyWatchPlanWithCancellation(ctx, plan, nil)
}
func (s *ScanSession) ApplyWatchPlanWithCancellation(ctx context.Context, plan WatchPlan, cancel *CancellationToken) (*ScanReport, error) {
	opts := s.options
	opts.Cancellation = cancel
	scanner, err := NewScanner(s.path, WithOptions(opts))
	if err != nil {
		return nil, err
	}
	update, err := scanner.ScanWatchPlanDetailed(ctx, s.report, plan)
	if err != nil {
		return nil, err
	}
	s.install(update.Report, update.Reason)
	return s.report, nil
}
func (s *ScanSession) install(report *ScanReport, reason WatchUpdateReason) {
	s.report, s.lastReason, s.hasReason = report, reason, true
	s.generation++
}
