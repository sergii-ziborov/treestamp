package treestamp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"regexp"
	"time"

	"github.com/sergii-ziborov/treestamp/internal/dx"
	"github.com/sergii-ziborov/treestamp/internal/scan"
)

func (p *Plan) Scan(ctx context.Context, root string) (*ScanReport, error) {
	s, err := p.scanner(root)
	if err != nil {
		return nil, err
	}
	rep, err := s.Scan(ctx)
	return p.finishLogged(ctx, "Scan", root, rep, err)
}

func (p *Plan) ScanCached(ctx context.Context, root string, cache *ScanCache) (*ScanReport, error) {
	s, err := p.scanner(root)
	if err != nil {
		return nil, err
	}
	rep, err := s.ScanCached(ctx, cache)
	return p.finishLogged(ctx, "ScanCached", root, rep, err)
}

func (p *Plan) ScanPaths(ctx context.Context, root string) ([]string, error) {
	if p.hashSet || p.binarySet || p.maxSet {
		return nil, &Error{Code: CodeUnsupported, Op: "ScanPaths", Err: errString("content options require Scan or EachFile")}
	}
	s, err := p.scanner(root)
	if err != nil {
		return nil, err
	}
	paths, err := s.ScanPaths(ctx)
	if err != nil {
		p.logFinish(ctx, "paths.finished", slog.Int("selected", len(paths)), reportStatus(nil, err))
		return paths, err
	}
	if err := ctx.Err(); err != nil {
		return paths, wrap(err, "ScanPaths", root)
	}
	p.logFinish(ctx, "paths.finished", slog.Int("selected", len(paths)), slog.String("status", "complete"))
	return paths, nil
}

func (p *Plan) ScanFS(ctx context.Context, fsys fs.FS, root string) (*ScanReport, error) {
	if p == nil {
		return nil, &Error{Code: CodeInvalid, Op: "ScanFS", Err: errEmptyRoot}
	}
	rep, err := scan.FullFS(ctx, fsys, root, toScanOptions(p.opts))
	if err != nil {
		return nil, wrap(err, "ScanFS", root)
	}
	return p.finishLogged(ctx, "ScanFS", root, fromFull(rep), nil)
}

func (p *Plan) ScanPathsFS(ctx context.Context, fsys fs.FS, root string) ([]string, error) {
	if p.hashSet || p.binarySet || p.maxSet {
		return nil, &Error{Code: CodeUnsupported, Op: "ScanPathsFS", Err: errString("content options require ScanFS or EachFileFS")}
	}
	paths, err := scan.PathsFS(ctx, fsys, root, toScanOptions(p.opts))
	if err != nil {
		err = wrap(err, "ScanPathsFS", root)
		p.logFinish(ctx, "paths.finished", slog.Int("selected", len(paths)), reportStatus(nil, err))
		return paths, err
	}
	if err := ctx.Err(); err != nil {
		return paths, wrap(err, "ScanPathsFS", root)
	}
	p.logFinish(ctx, "paths.finished", slog.Int("selected", len(paths)), slog.String("status", "complete"))
	return paths, nil
}

func (p *Plan) EachFileFS(ctx context.Context, fsys fs.FS, root string, consume func(ScannedFile, []byte) error) (*ScanSummary, error) {
	if p == nil {
		return nil, &Error{Code: CodeInvalid, Op: "EachFileFS", Err: errEmptyRoot}
	}
	start := time.Now()
	var n, read uint64
	var pace dx.Pace
	inner, err := scan.VisitOwnedFS(ctx, fsys, root, toScanOptions(p.opts), func(file scan.ScannedFile, data []byte) error {
		n++
		read += uint64(len(data))
		noteProgress(p.progress, &pace, start, n, read)
		return consume(publicFile(file), data)
	})
	return finishEach(ctx, p, root, inner, err)
}

func (p *Plan) EachFile(ctx context.Context, root string, consume func(ScannedFile, []byte) error) (*ScanSummary, error) {
	if p == nil {
		return nil, &Error{Code: CodeInvalid, Op: "EachFile", Err: errEmptyRoot}
	}
	start := time.Now()
	var n, read uint64
	var pace dx.Pace
	inner, err := scan.VisitOwned(ctx, root, toScanOptions(p.opts), func(file scan.ScannedFile, data []byte) error {
		n++
		read += uint64(len(data))
		noteProgress(p.progress, &pace, start, n, read)
		return consume(publicFile(file), data)
	})
	return finishEach(ctx, p, root, inner, err)
}

func (p *Plan) Explain(root, relative string) (PathExplanation, error) {
	s, err := p.scanner(root)
	if err != nil {
		return PathExplanation{}, err
	}
	return s.Explain(relative)
}

func (p *Plan) finishLogged(ctx context.Context, op, root string, rep *ScanReport, err error) (*ScanReport, error) {
	rep, err = finishScan(ctx, op, root, rep, err)
	if p.requireCache && (rep == nil || rep.Cache.Rebuilt) {
		return rep, &Error{Code: CodeCache, Op: op, Path: root, Err: errString("cache required")}
	}
	if rep != nil && rep.Cache.Rebuilt {
		dx.LogEvent(ctx, p.log, slog.LevelWarn, "cache.rebuilt", "cache.format_mismatch", "full_scan")
	}
	p.logFinish(ctx, "scan.finished", reportStatus(rep, err))
	return rep, err
}

func (p *Plan) Files(ctx context.Context, root string) func(func(ScannedFile, error) bool) {
	return func(yield func(ScannedFile, error) bool) {
		s, err := p.scanner(root)
		if err != nil {
			yield(ScannedFile{}, err)
			return
		}
		stop := false
		stream, err := s.ScanInto(ctx, ScanSinkFunc(func(f *ScannedFile) ScanSinkControl {
			if !yield(*f, nil) {
				stop = true
				return ScanSinkStop
			}
			return ScanSinkContinue
		}))
		if ferr := finishStream(ctx, "Files", root, stream, err, stop); ferr != nil {
			yield(ScannedFile{}, ferr)
		}
	}
}

func (p *Plan) WriteDiagnostics(w io.Writer, sum *ScanSummary) error {
	text := ""
	if sum != nil {
		text = sum.String()
	}
	return dx.WriteBundle(w, "treestamp", p.Describe(), text, []string{"config", "partial", "cancelled", "permission"})
}

func ScanIntoErr(ctx context.Context, root string, fn func(*ScannedFile) error, opts ...Option) (*ScanStreamReport, error) {
	p, err := Compile(opts...)
	if err != nil {
		return nil, err
	}
	s, err := p.scanner(root)
	if err != nil {
		return nil, err
	}
	var sinkErr error
	rep, err := s.ScanInto(ctx, ScanSinkFunc(func(f *ScannedFile) ScanSinkControl {
		if fn == nil {
			return ScanSinkContinue
		}
		if e := fn(f); e != nil {
			sinkErr = e
			return ScanSinkStop
		}
		return ScanSinkContinue
	}))
	if sinkErr != nil {
		return rep, &Error{Code: CodeCallback, Op: "ScanInto", Path: root, Err: sinkErr}
	}
	return rep, finishStream(ctx, "ScanInto", root, rep, err, false)
}

func (s ScanSummary) LogValue() slog.Value {
	return slog.GroupValue(slog.Int("selected", s.SelectedFiles), slog.Int("skipped", s.RecordedSkips), slog.Bool("complete", s.Complete))
}

func noteProgress(fn func(Progress), pace *dx.Pace, start time.Time, n, read uint64) {
	if fn == nil || !pace.Allow(time.Now()) {
		return
	}
	fn(Progress{Phase: "each", Processed: n, Bytes: read, Elapsed: time.Since(start)})
}

func finishScan(ctx context.Context, op, root string, rep *ScanReport, err error) (*ScanReport, error) {
	if err != nil {
		return nil, wrap(err, op, root)
	}
	if ferr := dx.Check(ctx, reportOutcome(rep)); ferr != nil {
		return rep, wrap(ferr, op, root)
	}
	return rep, nil
}

func finishEach(ctx context.Context, p *Plan, root string, inner *scan.ContentVisitReport, err error) (*ScanSummary, error) {
	var pub *ContentVisitReport
	if inner != nil {
		pub = fromContentReport(inner)
	}
	sum := visitSummary(pub, inner)
	if errors.Is(err, ErrStop) {
		sum.Stopped, sum.Complete = true, false
		p.logFinish(ctx, "each.finished", slog.String("status", "stopped"))
		return &sum, ErrStop
	}
	if err != nil {
		return &sum, &Error{Code: CodeCallback, Op: "EachFile", Path: root, Err: err}
	}
	if ferr := dx.Check(ctx, visitOutcome(pub)); ferr != nil {
		sum.Complete = false
		p.logFinish(ctx, "each.finished", slog.String("status", "partial"))
		return &sum, wrap(ferr, "EachFile", root)
	}
	p.logFinish(ctx, "each.finished", slog.Int("selected", sum.SelectedFiles), slog.String("status", "complete"))
	return &sum, nil
}

func visitSummary(rep *ContentVisitReport, inner *scan.ContentVisitReport) ScanSummary {
	if rep == nil {
		return ScanSummary{}
	}
	sum := ScanSummary{
		SelectedFiles: int(rep.Completed), RecordedSkips: len(rep.Skipped), Warnings: len(rep.Warnings),
		IgnoreSources: len(rep.IgnoreSources), Complete: rep.Complete && !rep.Stopped, Stopped: rep.Stopped,
		Termination: rep.Termination, Portable: rep.Portable, Cache: rep.Cache, SkippedByKind: map[SkipKind]int{},
	}
	for _, skipped := range rep.Skipped {
		sum.SkippedByKind[skipped.Kind]++
	}
	countOwned(&sum, inner)
	return sum
}

func countOwned(sum *ScanSummary, inner *scan.ContentVisitReport) {
	if inner == nil || inner.Manifest == nil {
		return
	}
	for _, file := range inner.Manifest.Files {
		sum.SelectedBytes += file.Bytes
		if file.ContentHash() != "" {
			sum.HashedFiles++
		}
		if file.Content != nil && file.Content.BinaryChecked {
			sum.BinaryCheckedFiles++
		}
	}
}

func finishStream(ctx context.Context, op, root string, stream *ScanStreamReport, err error, stopped bool) error {
	if stopped {
		return nil
	}
	if err != nil {
		return wrap(err, op, root)
	}
	if ferr := dx.Check(ctx, streamOutcome(stream)); ferr != nil {
		return wrap(ferr, op, root)
	}
	return nil
}

func reportOutcome(rep *ScanReport) dx.Outcome {
	if rep == nil {
		return dx.Outcome{}
	}
	return dx.Outcome{Complete: rep.Complete, Term: int(rep.Termination), Unread: unreadKinds(rep.Skipped)}
}

func visitOutcome(rep *ContentVisitReport) dx.Outcome {
	if rep == nil {
		return dx.Outcome{}
	}
	return dx.Outcome{Complete: rep.Complete && !rep.Stopped, Term: int(rep.Termination), Unread: unreadKinds(rep.Skipped)}
}

func streamOutcome(rep *ScanStreamReport) dx.Outcome {
	if rep == nil {
		return dx.Outcome{}
	}
	return dx.Outcome{Complete: rep.Complete && !rep.Stopped, Term: int(rep.Termination), Unread: unreadKinds(rep.Skipped)}
}

func unreadKinds(skipped []SkippedEntry) bool {
	for _, item := range skipped {
		if item.Kind == SkipIOError || item.Kind == SkipConcurrentModification {
			return true
		}
	}
	return false
}

func reportStatus(rep *ScanReport, err error) slog.Attr {
	if errors.Is(err, ErrPartial) {
		return slog.String("status", "partial")
	}
	if err != nil {
		return slog.String("status", "error")
	}
	if rep != nil && !rep.Complete {
		return slog.String("status", "limited")
	}
	return slog.String("status", "complete")
}

func cloneOptions(o Options) Options {
	o.Cancellation = nil
	o.IgnoreFiles, o.OverrideRules, o.Extensions = dx.Clone(o.IgnoreFiles), dx.Clone(o.OverrideRules), dx.Clone(o.Extensions)
	o.Filters = cloneFilters(o.Filters)
	if o.Limits.MaxEntries != nil {
		v := *o.Limits.MaxEntries
		o.Limits.MaxEntries = &v
	}
	if o.Limits.MaxTotalBytes != nil {
		v := *o.Limits.MaxTotalBytes
		o.Limits.MaxTotalBytes = &v
	}
	if o.Walk.MaxDepth != nil {
		v := *o.Walk.MaxDepth
		o.Walk.MaxDepth = &v
	}
	return o
}

func WithScope(globs ...string) Option {
	return func(b *planBuilder) error {
		b.opts.Filters.LocationInclude = append(b.opts.Filters.LocationInclude, globs...)
		return nil
	}
}

func WithLocationExclude(globs ...string) Option {
	return func(b *planBuilder) error {
		b.opts.Filters.LocationExclude = append(b.opts.Filters.LocationExclude, globs...)
		return nil
	}
}

func WithNoIgnore() Option {
	return func(b *planBuilder) error {
		b.opts.IgnoreFiles = nil
		return nil
	}
}

func cloneFilters(f Filters) Filters {
	f.IncludeNames, f.ExcludeNames = dx.Clone(f.IncludeNames), dx.Clone(f.ExcludeNames)
	f.IncludeDirs, f.ExcludeDirs = dx.Clone(f.IncludeDirs), dx.Clone(f.ExcludeDirs)
	f.ExcludeExtensions, f.LocationExclude = dx.Clone(f.ExcludeExtensions), dx.Clone(f.LocationExclude)
	f.LocationInclude = dx.Clone(f.LocationInclude)
	f.IncludeNameRegex = append([]*regexp.Regexp(nil), f.IncludeNameRegex...)
	f.ExcludeNameRegex = append([]*regexp.Regexp(nil), f.ExcludeNameRegex...)
	f.IncludeDirRegex = append([]*regexp.Regexp(nil), f.IncludeDirRegex...)
	f.ExcludeDirRegex = append([]*regexp.Regexp(nil), f.ExcludeDirRegex...)
	return f
}
