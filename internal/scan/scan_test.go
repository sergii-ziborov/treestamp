package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sergii-ziborov/treestamp/internal/runtime"
	"github.com/sergii-ziborov/treestamp/internal/selection"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

func TestFullPathsCompactCacheWatch(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".gitignore"), "secret.bin\n")
	mustWrite(t, filepath.Join(root, "keep.go"), "package keep\n")
	mustWrite(t, filepath.Join(root, "secret.bin"), "hidden")
	mustMkdir(t, filepath.Join(root, "node_modules"))
	mustWrite(t, filepath.Join(root, "node_modules", "x.js"), "x")
	ctx := context.Background()
	opts := DefaultOptions()
	paths, err := Paths(ctx, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(paths, "\n")
	if strings.Contains(joined, "secret.bin") || strings.Contains(joined, "node_modules") {
		t.Fatalf("leaked %v", paths)
	}
	if !contains(paths, "keep.go") {
		t.Fatalf("missing keep.go %v", paths)
	}
	report, err := Full(ctx, root, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Revision == "" || report.Descriptor.Version != 2 {
		t.Fatalf("report %+v", report.Descriptor)
	}
	compact, err := Compact(ctx, root, opts)
	if err != nil || len(compact.Files) == 0 {
		t.Fatalf("compact %v %v", compact, err)
	}
	cache := CacheFromReport(report)
	if !cache.Compatible(report.Root) || len(cache.Entries) == 0 {
		t.Fatalf("cache %+v", cache)
	}
	cached, err := Cached(ctx, root, opts, &cache)
	if err != nil || cached.Cache.ReusedHashes == 0 {
		t.Fatalf("cached %+v %v", cached, err)
	}
	inc, err := Incremental(ctx, root, opts, report)
	if err != nil {
		t.Fatal(err)
	}
	plan := WatchPlan{Changed: []string{"keep.go"}}
	update, err := Watch(ctx, root, opts, inc, plan)
	if err != nil {
		t.Fatal(err)
	}
	if update.Reason.String() == "" {
		t.Fatal("reason")
	}
}

func TestVisitContentAndInto(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "hello")
	ctx := context.Background()
	opts := DefaultOptions()
	var chunks int
	report, err := VisitContent(ctx, root, opts, VisitRevision, func(int) ContentVisitor {
		return func(ev ContentVisitEvent) ContentVisitControl {
			if ev.Kind == ContentChunk {
				chunks++
			}
			return ContentContinue
		}
	})
	if err != nil || report.Completed != 1 || chunks == 0 {
		t.Fatalf("visit %+v chunks=%d err=%v", report, chunks, err)
	}
	var seen int
	stream, err := Into(ctx, root, opts, func(*ScannedFile) StreamControl {
		seen++
		return SinkContinue
	})
	if err != nil || stream.Emitted != 1 || seen != 1 {
		t.Fatalf("into %+v seen=%d err=%v", stream, seen, err)
	}
}

func TestLimitsCancelAndBinary(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".gitignore"), "z.tmp\n")
	mustWrite(t, filepath.Join(root, "ok.txt"), "abc")
	mustWrite(t, filepath.Join(root, "nul.bin"), "a\x00b")
	mustWrite(t, filepath.Join(root, "z.tmp"), "ignored")
	opts := DefaultOptions()
	opts.MaxFileBytes = 50
	report, err := Full(context.Background(), root, opts)
	if err != nil {
		t.Fatal(err)
	}
	var sawBinary bool
	for _, s := range report.Skipped {
		if s.Kind == selection.SkipBinary {
			sawBinary = true
		}
	}
	if !sawBinary {
		t.Fatalf("skips %+v", report.Skipped)
	}
	compact, err := Compact(context.Background(), root, opts)
	if err != nil || len(compact.Skipped) < 2 || compact.Skipped[0].Relative > compact.Skipped[1].Relative {
		t.Fatalf("compact skip order: %+v %v", compact, err)
	}
	token := &runtime.Token{}
	token.Cancel()
	opts.Cancel = token
	limited, err := Full(context.Background(), root, opts)
	if err != nil {
		t.Fatal(err)
	}
	if limited.Termination != TermCancelled {
		t.Fatalf("term %v", limited.Termination)
	}
	opts.Cancel = nil
	n := uint64(0)
	opts.Limits.MaxEntries = &n
	opts.Limits.Timeout = time.Hour
	hit, err := Paths(context.Background(), root, opts)
	if err != ErrIncomplete || len(hit) != 0 {
		t.Fatalf("max entries %v %v", hit, err)
	}
	if TermCancelled.String() != "cancelled" || TermNone.String() != "" {
		t.Fatal(TermCancelled.String())
	}
}

func TestDescriptorMatchesAndEmptyRoot(t *testing.T) {
	opts := DefaultOptions()
	d := DescriptorFromOptions(opts)
	if !d.Matches(opts) {
		t.Fatal("match")
	}
	if d.Policy != "sha256:449649a73960fc2a8fe0efb28495e75e18e03b614e5c107b0efc147dac7786f9" {
		t.Fatalf("oracle descriptor: %s", d.Policy)
	}
	opts.HashFileContents = false
	if d.Matches(opts) {
		t.Fatal("should differ")
	}
	if _, err := Full(context.Background(), "", DefaultOptions()); err == nil {
		t.Fatal("empty root")
	}
	if _, err := Full(context.Background(), filepath.Join(t.TempDir(), "missing"), DefaultOptions()); err == nil {
		t.Fatal("missing")
	}
}

func TestCacheInvalidateAndWatchEarly(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	report, err := Full(context.Background(), root, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	cache := CacheFromReport(report)
	if cache.Invalidate([]string{"a.txt"}) != 1 {
		t.Fatal("invalidate")
	}
	update, err := Watch(context.Background(), root, DefaultOptions(), nil, WatchPlan{})
	if err != nil {
		t.Fatal(err)
	}
	if update.Reason != WatchFullIncomplete {
		t.Fatalf("reason %v", update.Reason)
	}
	full, err := Watch(context.Background(), root, DefaultOptions(), report, WatchPlan{FullRescan: true})
	if err != nil || full.Reason != WatchFullStructural {
		t.Fatalf("full %v %v", full, err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func contains(paths []string, name string) bool {
	for _, p := range paths {
		if p == name || strings.HasSuffix(p, "/"+name) {
			return true
		}
	}
	return false
}

func TestLimitsCancelBinaryAndDescriptor(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "ok.txt"), "ok")
	mustWrite(t, filepath.Join(root, "nul.bin"), "a\x00b")
	opts := DefaultOptions()
	one := uint64(1)
	opts.Limits.MaxEntries = &one
	report, err := Full(context.Background(), root, opts)
	if err != nil || report.Termination != TermMaxEntries {
		t.Fatalf("limit %+v %v", report, err)
	}
	d := DescriptorFromOptions(opts)
	if !d.Matches(opts) || d.Version != 2 {
		t.Fatal("desc")
	}
	compact, err := Compact(context.Background(), root, DefaultOptions())
	if err != nil || len(compact.Files) == 0 {
		t.Fatal(err)
	}
	_, _ = Incremental(context.Background(), root, DefaultOptions(), report)
	_, _ = Cached(context.Background(), root, DefaultOptions(), nil)
}

func TestVisitStreamingQuitAndCacheCompact(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "hello")
	mustWrite(t, filepath.Join(root, "b.txt"), "world")
	opts := DefaultOptions()
	report, err := VisitContent(context.Background(), root, opts, VisitStreaming, func(int) ContentVisitor {
		return func(ev ContentVisitEvent) ContentVisitControl {
			if ev.Kind == ContentFileStart {
				return ContentQuit
			}
			return ContentContinue
		}
	})
	if err != nil || !report.Stopped {
		t.Fatalf("quit %+v %v", report, err)
	}
	full, err := Full(context.Background(), root, opts)
	if err != nil {
		t.Fatal(err)
	}
	compactCache := CacheFromCompact(&CompactReport{Root: full.Root, Files: []CompactFile{{
		Relative: full.Files[0].Relative, Bytes: full.Files[0].Bytes,
		Content: &ContentEvidence{ContentHash: full.Files[0].ContentHash, ContentFingerprint: full.Files[0].ContentFingerprint, Version: full.Files[0].Version, BinaryChecked: true},
	}}})
	if !compactCache.Compatible(full.Root) {
		t.Fatal("compact cache")
	}
	n := compactCache.Invalidate([]string{"missing"})
	_ = n
	stream, err := Into(context.Background(), root, opts, func(*ScannedFile) StreamControl { return SinkStop })
	if err != nil || !stream.Stopped {
		t.Fatalf("stop %+v %v", stream, err)
	}
}

func TestParallelInspectStrictCacheAndFullWatch(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "hello")
	mustWrite(t, filepath.Join(root, "b.txt"), "world")
	opts := DefaultOptions()
	opts.ContentWorkers = 2
	opts.CacheValidation = CacheStrict
	first, err := Full(context.Background(), root, opts)
	if err != nil {
		t.Fatal(err)
	}
	cache := CacheFromReport(first)
	opts.Cache = &cache
	second, err := Cached(context.Background(), root, opts, &cache)
	if err != nil || second.Cache.ReusedHashes == 0 {
		t.Fatalf("strict %+v %v", second, err)
	}
	other := t.TempDir()
	mustWrite(t, filepath.Join(other, "c.txt"), "c")
	update, err := Watch(context.Background(), other, DefaultOptions(), first, WatchPlan{Changed: []string{"c.txt"}})
	if err != nil || update.Reason != WatchFullStructural {
		t.Fatalf("watch %+v %v", update, err)
	}
	_ = errNotDir.Error()
	_ = TermMaxTotalBytes.String()
	_ = TermTimeout.String()
	_ = TermCancelled.String()
	_ = Termination(99).String()
	_ = WatchFullPolicy.String()
	_ = WatchFullIgnore.String()
	_ = WatchFullIncomplete.String()
	_ = WatchReason(99).String()
	_ = CompactFile{Relative: "x", Bytes: 1}.ContentHash()
	git := DefaultOptions()
	git.IgnorePolicy.GitGlobal = true
	_ = portablePolicy(git)
	lim := DefaultOptions()
	zero := uint64(0)
	lim.Limits.MaxTotalBytes = &zero
	_, _ = Full(context.Background(), root, lim)
}

func TestDiscoveryWatchVisitEdges(t *testing.T) {
	if _, err := Full(context.Background(), "", DefaultOptions()); err == nil {
		t.Fatal("empty")
	}
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "hello")
	mustWrite(t, filepath.Join(root, ".hid.txt"), "h")
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "sub", ".gitignore"), "{\n")
	mustWrite(t, filepath.Join(root, "sub", "b.txt"), "b")
	mustWrite(t, filepath.Join(root, "nul.bin"), "a\x00b")
	if _, err := Full(context.Background(), filepath.Join(root, "a.txt"), DefaultOptions()); err == nil {
		t.Fatal("file root")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if report, err := Full(ctx, root, DefaultOptions()); err != nil || report.Termination != TermCancelled {
		t.Fatalf("cancel %+v %v", report, err)
	}
	opts := DefaultOptions()
	opts.SkipHidden = true
	opts.Limits.Timeout = time.Hour
	opts.Walk.ErrorPolicy = walk.ErrorContinue
	if _, err := Full(context.Background(), root, opts); err != nil {
		t.Fatal(err)
	}
	tok := &runtime.Token{}
	tok.Cancel()
	opts.Cancel = tok
	if report, err := Full(context.Background(), root, opts); err != nil || report.Termination != TermCancelled {
		t.Fatalf("token %+v %v", report, err)
	}
	first, err := Full(context.Background(), root, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Watch(context.Background(), root, DefaultOptions(), first, WatchPlan{FullRescan: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := Watch(context.Background(), root, DefaultOptions(), nil, WatchPlan{Changed: []string{"a.txt"}}); err != nil {
		t.Fatal(err)
	}
	changed := *first
	changed.Complete, changed.Termination = true, TermNone
	changed.Descriptor.Policy = "changed"
	if upd, err := Watch(context.Background(), root, DefaultOptions(), &changed, WatchPlan{Changed: []string{"a.txt"}}); err != nil || upd.Reason != WatchFullPolicy {
		t.Fatalf("policy %+v %v", upd, err)
	}
	_, _ = VisitContent(context.Background(), root, DefaultOptions(), VisitRevision, func(int) ContentVisitor {
		return func(ev ContentVisitEvent) ContentVisitControl {
			if ev.Kind == ContentChunk {
				return ContentSkipFile
			}
			if ev.Kind == ContentFileEnd {
				return ContentQuit
			}
			return ContentContinue
		}
	})
	_, _ = VisitContent(context.Background(), root, DefaultOptions(), VisitStreaming, func(int) ContentVisitor {
		return func(ev ContentVisitEvent) ContentVisitControl {
			if ev.Kind == ContentChunk {
				return ContentQuit
			}
			return ContentContinue
		}
	})
	_, _ = Into(context.Background(), "", DefaultOptions(), func(*ScannedFile) StreamControl { return SinkContinue })
}

func TestVisitChangedOnlyPlanFiles(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.go"), "package keep\n")
	mustWrite(t, filepath.Join(root, "other.go"), "package other\n")
	var seen []string
	report, err := VisitChanged(context.Background(), root, DefaultOptions(), WatchPlan{Changed: []string{"keep.go", "../escape", "missing.go"}}, func(int) ContentVisitor {
		return func(ev ContentVisitEvent) ContentVisitControl {
			if ev.Kind == ContentFileStart {
				seen = append(seen, ev.File.Relative)
			}
			return ContentContinue
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0] != "keep.go" {
		t.Fatalf("visited %v", seen)
	}
	var ioSkip, escapeSkip bool
	for _, s := range report.Skipped {
		if s.Kind == selection.SkipIOError {
			ioSkip = true
		}
		if s.Kind == selection.SkipPathEscape {
			escapeSkip = true
		}
	}
	if !ioSkip || !escapeSkip {
		t.Fatalf("skips %+v", report.Skipped)
	}
}

func TestVisitChangedHonorsNestedIgnore(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "sub", ".gitignore"), "*.tmp\n")
	mustWrite(t, filepath.Join(root, "sub", "drop.tmp"), "tmp")
	mustWrite(t, filepath.Join(root, "keep.go"), "package keep\n")
	var seen []string
	report, err := VisitChanged(context.Background(), root, DefaultOptions(), WatchPlan{Changed: []string{"sub/drop.tmp"}}, func(int) ContentVisitor {
		return func(ev ContentVisitEvent) ContentVisitControl {
			if ev.Kind == ContentFileStart {
				seen = append(seen, ev.File.Relative)
			}
			return ContentContinue
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 0 {
		t.Fatalf("changed visit leaked ignored file %v", seen)
	}
	full, err := Full(context.Background(), root, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range full.Files {
		if file.Relative == "sub/drop.tmp" {
			t.Fatal("full selected ignored tmp")
		}
	}
	_ = report
}

func TestPathsTimeoutIsError(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	opts := DefaultOptions()
	opts.Limits.Timeout = time.Nanosecond
	opts.Started = time.Now().Add(-time.Second)
	if _, err := Paths(context.Background(), root, opts); err == nil {
		t.Fatal("timeout must fail ScanPaths")
	}
}

func TestWatchReplacesPrefixEvidenceAndRejectsEscape(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.go"), "package keep\n")
	mustWrite(t, filepath.Join(root, "stale.go"), "package stale\n")
	first, err := Full(context.Background(), root, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	first.Skipped = append(first.Skipped, Skipped{Relative: "keep.go", Kind: selection.SkipIOError, Detail: "old"})
	first.Skipped = append(first.Skipped, Skipped{Relative: "other.go", Kind: selection.SkipIOError, Detail: "keep"})
	first.Warnings = []Warning{{Relative: "keep.go", Message: "old"}, {Relative: "other.go", Message: "keep"}}
	update, err := Watch(context.Background(), root, DefaultOptions(), first, WatchPlan{Changed: []string{"keep.go", "../secret"}})
	if err != nil || update.Reason != WatchIncremental {
		t.Fatalf("watch %+v %v", update, err)
	}
	if update.Report == nil {
		t.Fatal("report")
	}
	for _, s := range update.Report.Skipped {
		if s.Relative == "keep.go" && s.Detail == "old" {
			t.Fatal("stale skip kept")
		}
		if s.Kind == selection.SkipPathEscape && s.Relative == "../secret" {
			t.Fatal("escaped path recorded as a live relative")
		}
	}
	var keptOther bool
	for _, s := range update.Report.Skipped {
		if s.Relative == "other.go" {
			keptOther = true
		}
	}
	if !keptOther {
		t.Fatalf("unrelated skip dropped %+v", update.Report.Skipped)
	}
	missing, err := Watch(context.Background(), root, DefaultOptions(), first, WatchPlan{Changed: []string{"gone.go"}})
	if err != nil {
		t.Fatal(err)
	}
	var sawIO bool
	for _, s := range missing.Report.Skipped {
		if s.Relative == "gone.go" && s.Kind == selection.SkipIOError {
			sawIO = true
		}
	}
	if !sawIO {
		t.Fatalf("missing %+v", missing.Report.Skipped)
	}
	if missing.Report.Complete {
		t.Fatal("io skip must not look complete")
	}
}

func TestWatchKeepsIncrementalWithNestedIgnores(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "other"))
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "other", ".gitignore"), "*.log\n")
	mustWrite(t, filepath.Join(root, "sub", ".gitignore"), "*.tmp\n")
	mustWrite(t, filepath.Join(root, "keep.go"), "package keep\n")
	mustWrite(t, filepath.Join(root, "sub", "ok.go"), "package ok\n")
	mustWrite(t, filepath.Join(root, "sub", "drop.tmp"), "tmp")
	mustWrite(t, filepath.Join(root, "other", "noise.log"), "log")
	opts := DefaultOptions()
	first, err := Full(context.Background(), root, opts)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "sub", "ok.go"), "package ok\n// changed\n")
	update, err := Watch(context.Background(), root, opts, first, WatchPlan{Changed: []string{"sub/ok.go"}})
	if err != nil || update.Reason != WatchIncremental {
		t.Fatalf("reason %v %v", update, err)
	}
	fresh, err := Full(context.Background(), root, opts)
	if err != nil {
		t.Fatal(err)
	}
	if selected(update.Report) != selected(fresh) {
		t.Fatalf("watch %v full %v", selected(update.Report), selected(fresh))
	}
}

func TestWatchPolicyChangeForcesRescan(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "keep.go"), "package keep\n")
	mustWrite(t, filepath.Join(root, "drop.go"), "package drop\n")
	first, err := Full(context.Background(), root, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	next := DefaultOptions()
	next.Filters.ExcludeNames = []string{"drop.go"}
	update, err := Watch(context.Background(), root, next, first, WatchPlan{Changed: []string{"keep.go"}})
	if err != nil || update.Reason != WatchFullPolicy {
		t.Fatalf("policy %v %v", update, err)
	}
}

func TestZeroByteCapAndIncompatibleCache(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "hello")
	opts := DefaultOptions()
	opts.MaxFileBytesZero = true
	opts.MaxFileBytes = 0
	report, err := Full(context.Background(), root, opts)
	if err != nil {
		t.Fatal(err)
	}
	var over bool
	for _, skipped := range report.Skipped {
		if skipped.Kind == selection.SkipOversized {
			over = true
		}
	}
	if !over {
		t.Fatalf("zero cap %+v files=%v", report.Skipped, report.Files)
	}
	open := DefaultOptions()
	open.MaxFileBytes = 0
	unlimited, err := Full(context.Background(), root, open)
	if err != nil || len(unlimited.Files) == 0 {
		t.Fatalf("unlimited %v %v", unlimited, err)
	}
	bad := &Cache{FormatVersion: 1, Root: root}
	cached, err := Cached(context.Background(), root, DefaultOptions(), bad)
	if err != nil || cached == nil || !cached.Cache.Rebuilt {
		t.Fatalf("rebuilt %+v %v", cached, err)
	}
}

func TestVisitOwnedStopsWithoutReplay(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustWrite(t, filepath.Join(root, "b.txt"), "b")
	var n int
	_, err := VisitOwned(context.Background(), root, DefaultOptions(), func(ScannedFile, []byte) error {
		n++
		return errors.New("once")
	})
	if err == nil || n != 1 {
		t.Fatalf("replay n=%d err=%v", n, err)
	}
}

func selected(report *Report) string {
	var out []string
	for _, file := range report.Files {
		out = append(out, file.Relative)
	}
	return strings.Join(out, ",")
}
