package treestamp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/sergii-ziborov/treestamp"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

func TestScanPathsRespectsGitignoreAndSkipsHash(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".gitignore"), "secret.bin\n")
	mustWriteFile(t, filepath.Join(root, "keep.go"), "package keep\n")
	mustWriteFile(t, filepath.Join(root, "secret.bin"), "hidden")
	mustMkdir(t, filepath.Join(root, "node_modules"))
	mustWriteFile(t, filepath.Join(root, "node_modules", "x.js"), "x")

	ctx := context.Background()
	paths, err := ScanPaths(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(paths, "\n")
	if strings.Contains(joined, "secret.bin") {
		t.Fatalf("gitignore leaked: %v", paths)
	}
	if strings.Contains(joined, "node_modules") {
		t.Fatalf("standard skip leaked: %v", paths)
	}
	if !containsRel(paths, "keep.go") {
		t.Fatalf("missing keep.go: %v", paths)
	}

	opts := DefaultOptions()
	opts.HashFileContents = false
	opts.DetectBinaryFiles = false
	scanner, err := NewScanner(root, WithOptions(opts))
	if err != nil {
		t.Fatal(err)
	}
	report, err := scanner.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Revision == "" || !strings.HasPrefix(report.Descriptor.Policy, "sha256:") {
		t.Fatalf("revision/descriptor: %+v", report.Descriptor)
	}
	if report.Descriptor.Version != 2 {
		t.Fatalf("descriptor version %d", report.Descriptor.Version)
	}
}

func TestScanHashesAndSkipsBinaryAndOversize(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "ok.txt"), "abc")
	mustWriteFile(t, filepath.Join(root, "nul.bin"), "a\x00b")
	big := bytes.Repeat([]byte("x"), 200)
	mustWriteFile(t, filepath.Join(root, "big.txt"), string(big))

	opts := DefaultOptions()
	opts.MaxFileBytes = 50
	scanner, err := NewScanner(root, WithOptions(opts))
	if err != nil {
		t.Fatal(err)
	}
	report, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var hashed bool
	for _, f := range report.Files {
		if f.Relative == "ok.txt" {
			if !strings.HasPrefix(f.ContentHash, "sha256:") {
				t.Fatalf("hash %q", f.ContentHash)
			}
			hashed = true
		}
		if f.Relative == "big.txt" || f.Relative == "nul.bin" {
			t.Fatalf("should be skipped: %s", f.Relative)
		}
	}
	if !hashed {
		t.Fatal("ok.txt not selected")
	}
	var sawBinary, sawOver bool
	for _, s := range report.Skipped {
		if s.Kind == SkipBinary {
			sawBinary = true
		}
		if s.Kind == SkipOversized {
			sawOver = true
		}
	}
	if !sawBinary || !sawOver {
		t.Fatalf("skips=%+v", report.Skipped)
	}

	paths, err := ScanPaths(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !containsRel(paths, "big.txt") {
		t.Fatal("ScanPaths must not apply max_file_bytes")
	}
}

func mustWriteFile(t *testing.T, path, body string) {
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

func containsRel(paths []string, name string) bool {
	for _, p := range paths {
		if p == name || strings.HasSuffix(p, "/"+name) {
			return true
		}
	}
	return false
}

func TestCacheAndIncrementalReuse(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.txt"), "abc")
	first, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	cache := first.ToCache()
	if !cache.IsCompatible(first.Root) || len(cache.Entries) != 1 {
		t.Fatalf("cache %+v", cache)
	}
	scanner, err := NewScanner(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := scanner.ScanCached(context.Background(), &cache)
	if err != nil || second.Cache.ReusedHashes == 0 || second.Revision != first.Revision {
		t.Fatalf("cached %+v %v", second, err)
	}
}

func TestWatchPlanIncremental(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "keep.txt"), "k")
	mustWriteFile(t, filepath.Join(root, "old.txt"), "o")
	first, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(root, "new.txt"), "n")
	if err := os.Remove(filepath.Join(root, "old.txt")); err != nil {
		t.Fatal(err)
	}
	adapter, err := NewWatcherEventAdapter(root, DefaultOptions().IgnoreFiles)
	if err != nil {
		t.Fatal(err)
	}
	plan := adapter.Plan([]WatchEvent{NewWatchEvent(filepath.Join(root, "new.txt"), WatchCreate), NewWatchEvent(filepath.Join(root, "old.txt"), WatchRemove)})
	if plan.FullRescan {
		t.Fatal("incremental")
	}
	scanner, err := NewScanner(root)
	if err != nil {
		t.Fatal(err)
	}
	update, err := scanner.ScanWatchPlanDetailed(context.Background(), first, plan)
	if err != nil || update.Reason.String() != "Incremental" {
		t.Fatalf("%v %v", update, err)
	}
}

func TestMultiScannerAndStateful(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	mustWriteFile(t, filepath.Join(a, "a.go"), "package a")
	mustWriteFile(t, filepath.Join(b, "b.go"), "package b")
	report, err := NewMultiScanner(a).AddRoot(b).Scan(context.Background())
	if err != nil || report.Len() != 2 {
		t.Fatalf("%v %v", report, err)
	}
	w, err := NewStatefulWalkBuilder[int, string](a, 1).ProcessReadDir(func(depth int, dir string, state *int, batch *[]walk.StatefulResult[string]) {
		for i := range *batch {
			if (*batch)[i].Entry != nil {
				(*batch)[i].Entry.State = "ok"
			}
		}
	}).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	found := false
	for {
		entry, err := w.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if entry.IsFile() && entry.State == "ok" {
			found = true
		}
	}
	if !found {
		t.Fatal("stateful")
	}
}

func TestKillerScanPathsIgnoresMaxFileBytes(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "small.txt"), "ok")
	mustWriteFile(t, filepath.Join(root, "big.bin"), strings.Repeat("x", 64))
	scanner, err := NewScanner(root, WithOptions(DefaultOptions().WithMaxFileBytes(8)))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := scanner.ScanPaths(context.Background())
	if err != nil || !strings.Contains(strings.Join(paths, ","), "big.bin") {
		t.Fatal(paths)
	}
	report, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range report.Files {
		if file.Relative == "big.bin" {
			t.Fatal("oversized")
		}
	}
}

func TestKillerScanIntoStopsWithoutFullManifest(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.txt"), "a")
	mustWriteFile(t, filepath.Join(root, "b.txt"), "b")
	scanner, err := NewScanner(root)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := scanner.ScanInto(context.Background(), ScanSinkFunc(func(*ScannedFile) ScanSinkControl { return ScanSinkStop }))
	if err != nil || !stream.Stopped || stream.Emitted != 1 {
		t.Fatalf("%+v %v", stream, err)
	}
}

func TestKillerCompactRoundTrip(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.txt"), "abc")
	full, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	compact, err := ScanCompact(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if compact.AbsolutePath(compact.Files[0]) != full.Files[0].AbsolutePath() || full.ToCompact().Revision != compact.Revision {
		t.Fatal("roundtrip")
	}
	if compact.IntoScanReport().Files[0].Absolute == "" {
		t.Fatal("into")
	}
}

func TestKillerSnapshotStale(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	mustWriteFile(t, path, "abc")
	report, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := report.ContentProvider()
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, path, "changed")
	if _, err := provider.Read("a.txt"); err == nil {
		t.Fatal("stale")
	}
}

func TestKillerOrderedPullMatchesSerial(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWriteFile(t, filepath.Join(root, "a.txt"), "a")
	mustWriteFile(t, filepath.Join(root, "sub", "b.txt"), "b")
	serial, err := NewWalker(root)
	if err != nil {
		t.Fatal(err)
	}
	defer serial.Close()
	var want []string
	for {
		entry, err := serial.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, filepath.ToSlash(entry.RelativePath()))
	}
	iter, err := NewParallelWalker(root).TryIntoIterOrderedBounded(2)
	if err != nil {
		t.Fatal(err)
	}
	defer iter.Close()
	var got []string
	for {
		entry, err := iter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, filepath.ToSlash(entry.RelativePath()))
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%v vs %v", got, want)
	}
}

func TestKillerSelectionMatcherAndTypes(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".gitignore"), "skip.txt\n")
	mustWriteFile(t, filepath.Join(root, "keep.go"), "package k")
	mustWriteFile(t, filepath.Join(root, "skip.txt"), "no")
	matcher, err := NewSelectionMatcher(root, DefaultOptions().WithFileTypes(DefaultFileTypes().WithGlobs("notes", "*.md").Select("go", "notes")))
	if err != nil {
		t.Fatal(err)
	}
	keep, err := matcher.Matched(filepath.Join(root, "keep.go"))
	if err != nil || !keep.IsSelected() {
		t.Fatal(err)
	}
	skip, err := matcher.Matched(filepath.Join(root, "skip.txt"))
	if err != nil || skip.SkipKind() != SkipIgnored {
		t.Fatal(err)
	}
}

func TestKillerFileWalkerAndSkipFiles(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWriteFile(t, filepath.Join(root, "a.go"), "package a")
	mustWriteFile(t, filepath.Join(root, "b.txt"), "b")
	mustWriteFile(t, filepath.Join(root, "sub", "c.go"), "package c")
	ch := make(chan *File, 8)
	if err := NewFileWalker(root, ch).AllowListExtensions("go").Start(); err != nil {
		t.Fatal(err)
	}
	var files []string
	for file := range ch {
		files = append(files, file.Filename)
	}
	joined := strings.Join(files, ",")
	if !strings.Contains(joined, "a.go") || strings.Contains(joined, "b.txt") {
		t.Fatal(files)
	}
}

func TestKillerDescriptorAndMetadataOnly(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.txt"), "abc")
	opts := DefaultOptions().MetadataOnly()
	first := DescriptorFromOptions(opts)
	if !first.Matches(opts) || first.Matches(DefaultOptions()) {
		t.Fatal("descriptor")
	}
	scanner, err := NewScanner(root, WithOptions(opts))
	if err != nil {
		t.Fatal(err)
	}
	report, err := scanner.Scan(context.Background())
	if err != nil || report.Files[0].ContentHash != "" {
		t.Fatal(err)
	}
}

func TestFacadeEdges(t *testing.T) {
	a := DefaultOptions()
	a.Filters.IncludeNames, a.Filters.ExcludeNames = []string{"x", "exc-namey"}, []string{"z"}
	b := DefaultOptions()
	b.Filters.IncludeNames, b.Filters.ExcludeNames = []string{"x"}, []string{"y", "exc-namez"}
	if DescriptorFromOptions(a).Policy == DescriptorFromOptions(b).Policy {
		t.Fatal("filter policy collision")
	}
	if _, err := NewScanner(".", WithOptions(DefaultOptions().WithIncludeFilenameRegex("["))); err == nil {
		t.Fatal("invalid regex")
	}
	if _, err := CompileFilters(nil, nil, nil, nil, []string{"["}, nil, nil, nil); err == nil {
		t.Fatal("compile filters")
	}
	if err := WalkDirs(t.TempDir(), DirWalkOptions{PostChildrenCallback: func(string, fs.DirEntry, error) error { return nil }, NumWorkers: 2}); err == nil {
		t.Fatal("unsupported hooks")
	}
	if _, err := NewScanner(""); err == nil {
		t.Fatal("empty")
	}
	for _, name := range DefaultOptions().IgnoreFiles {
		if name == ".treestampignore" {
			t.Fatal(name)
		}
	}
	if !IsSameOrDescendant("src/a.go", "src") || !PathCoveredByPrefixes("src/a", []string{"src"}) || len(CollapsePathPrefixes([]string{"src/nested", "src"})) != 1 {
		t.Fatal("path")
	}
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.go"), "package a")
	mustWriteFile(t, filepath.Join(root, "b.go"), "package b")
	tok := NewCancellationToken()
	tok.Cancel()
	if !tok.IsCancelled() {
		t.Fatal("cancel")
	}
	_ = NoneIgnorePolicy()
	_ = GitCompatibleIgnorePolicy().WithGitExclude(false).WithGitGlobal(false).WithRequireGit(false).WithExplicitFile("x")
	_ = DefaultOptions().WithIgnoreCase(true).WithParallelism(2)
	opts := DefaultOptions().WithIgnorePolicy(RepositoryIgnorePolicy().WithGitIgnore(true).WithDotIgnore(true).WithCustomIgnore(true).WithParentRules(false)).WithSkipHidden(true).WithStandardSkips(true).WithExtensions("go").WithOverrideRules("*.tmp").WithMaxEntries(10).WithMaxTotalBytes(1000).WithTimeout(0).WithCancellation(nil).WithCacheValidation(CacheValidationStrict).WithContentValidation(ContentValidationFast).WithContentDiscovery(ContentDiscoveryStreaming).WithIgnoreFiles(".gitignore", ".ignore", ".weavatrixignore").WithFileTypes(DefaultFileTypes()).WithTraversalParallelism(1).WithContentParallelism(1).SelectedFilesOnly()
	scanner, err := NewScanner(root, WithOptions(opts), WithTraversalWorkers(1), WithContentWorkers(1))
	if err != nil {
		t.Fatal(err)
	}
	scanner.Options(opts)
	full, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.ScanIncremental(context.Background(), full); err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.VisitContentStreaming(context.Background(), func(int) ContentVisitor {
		return func(ContentVisitEvent) ContentVisitControl { return ContentVisitContinue }
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.VisitContentManifest(context.Background(), func(int) ContentVisitor {
		return func(ContentVisitEvent) ContentVisitControl { return ContentVisitContinue }
	}); err != nil {
		t.Fatal(err)
	}
	changed, err := scanner.VisitChangedContent(context.Background(), WatchPlan{FullRescan: true}, nil)
	if err != nil || !changed.FullRescanRequired {
		t.Fatal(changed, err)
	}
	if _, err := scanner.VisitChangedContent(context.Background(), WatchPlan{}, func(int) ContentVisitor {
		return func(ContentVisitEvent) ContentVisitControl { return ContentVisitContinue }
	}); err != nil {
		t.Fatal(err)
	}
	adapter, err := NewWatcherEventAdapterWithOptions(root, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	plan := adapter.Plan([]WatchEvent{NewWatchEvent(filepath.Join(root, "a.go"), WatchModify), NewWatchEvent(root, WatchRescan)})
	_ = plan.Invalidated()
	_ = plan.InvalidatesPath("a.go")
	_ = plan.ExpandRemoved([]string{"a.go", "b.go"})
	if _, err := scanner.ScanWatchPlan(context.Background(), full, WatchPlan{Changed: []string{"a.go"}}); err != nil {
		t.Fatal(err)
	}
	session, err := OpenScanSession(context.Background(), root, DefaultOptions())
	if err != nil || session.Root() == "" || session.IntoReport() == nil {
		t.Fatal(err)
	}
	_, _ = session.LastUpdateReason()
	_ = session.Report()
	_ = session.Generation()
	if _, err := session.ApplyWatchPlan(context.Background(), WatchPlan{Changed: []string{"a.go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ApplyWatchPlanWithCancellation(context.Background(), WatchPlan{Changed: []string{"b.go"}}, nil); err != nil {
		t.Fatal(err)
	}
	_ = full.PortableFilesPage(0, 1)
	_ = full.PortableFilesPage(99, 1)
	_ = full.Delta(full)
	_ = full.ToPortable()
	_ = full.Summary()
	if _, err := session.FilesPage(1, 0, 10); err == nil {
		t.Fatal("stale page")
	}
	_, _ = session.Rescan(context.Background())
	compact := full.ToCompact()
	_ = compact.Summary().String()
	_ = compact.ToCache()
	cache := full.ToCache()
	_ = cache.Invalidate([]string{"a.go"})
	_ = cache.ApplyWatchPlan(WatchPlan{FullRescan: true})
	_, _ = scanner.ScanInto(context.Background(), ScanSinkFunc(func(*ScannedFile) ScanSinkControl { return ScanSinkContinue }))
	_, _ = scanner.VisitContent(context.Background(), func(int) ContentVisitor {
		return func(ContentVisitEvent) ContentVisitControl { return ContentVisitContinue }
	})
	iter := NewParallelWalker(root).IntoIterBounded(2)
	_, _ = iter.Next()
	iter.Close()
	hashed, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	_ = hashed.PortableFilesPage(0, 100)
	if ms, err := NewScanner(root, WithOptions(DefaultOptions().MetadataOnly())); err == nil {
		rep, _ := ms.Scan(context.Background())
		_ = DeltaBetween(rep, rep)
	}
	prov, err := hashed.ContentProvider()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prov.Open("a.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := (*ScanReport)(nil).ContentProvider(); err == nil {
		t.Fatal("nil snap")
	}
	matcher, err := NewRepositoryMatcher(root, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	_ = matcher.Match("a.go", false)
	_ = matcher.EnterDir(root, "")
	_ = matcher.Sources()
	typed := &Error{Code: CodeInvalid, Op: "t", Path: "p", Err: io.EOF}
	if errors.Unwrap(typed) == nil || typed.Error() == "" || IsNotImplemented(nil) {
		t.Fatal("err")
	}
	snapErr := &SnapshotReadError{Relative: "a.go", Reason: "stale", Err: io.EOF}
	_ = snapErr.Error()
	_ = snapErr.Unwrap()
	_ = ScanTermination(99).String()
	_ = WatchUpdateReason(99).String()
	_ = WatchReasonFullPolicy.AsString()
	_, _, _, _ = MultiContentVisitReport{}.Len(), MultiContentVisitReport{}.IsEmpty(), MultiScanReport{}.IsEmpty(), ScanDelta{}.IsEmpty()
	_ = (*Error)(nil).Error()
	_ = (*Error)(nil).Unwrap()
	_ = (&Error{Code: CodeInvalid, Op: "t", Path: "p"}).Error()
	_ = ScanSummary{Termination: TerminationTimeout, Cache: ScanCacheStats{ReusedHashes: 1, ContentReads: 1}, SkippedByKind: map[SkipKind]int{SkipBinary: 1}}.String()
	if _, err := (*Scanner)(nil).Scan(context.Background()); err == nil {
		t.Fatal("nil scanner")
	}
	if err := WalkUnsorted(root, func(string, os.DirEntry, error) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := WalkDefault(root, IgnoreDuplicateFiles(IgnoreDuplicateDirs(func(string, os.DirEntry, error) error { return nil }))); err != nil {
		t.Fatal(err)
	}
	_ = Walk(root, func(string, os.DirEntry, error) error { return SkipDir })
	_ = Walk(root, func(string, os.DirEntry, error) error { return SkipAll })
	_ = Walk(root, func(string, os.DirEntry, error) error { return ErrSkipFiles })
	_ = WalkWithConfig(root, Config{Follow: true, Sort: true, MaxDepth: 2, ToSlash: true}, func(string, os.DirEntry, error) error { return nil })
	_ = WalkWithConfig(root, Config{NumWorkers: 2}, func(string, os.DirEntry, error) error { return SkipDir })
	_ = WalkWithConfig(root, Config{NumWorkers: 2}, func(string, os.DirEntry, error) error { return SkipAll })
	_ = WalkWithConfig(root, Config{NumWorkers: 2}, func(string, os.DirEntry, error) error { return ErrSkipFiles })
	stopErr := errors.New("stop")
	if err := WalkWithConfig(root, Config{NumWorkers: 2}, func(string, os.DirEntry, error) error { return stopErr }); !errors.Is(err, stopErr) {
		t.Fatal(err)
	}
	w, err := NewWalkerWithOptions(root, DefaultWalkOptions())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = CollectWalk(w.Next)
	w.Close()
	_, _ = CollectParallel(root, 1, true)
	_ = WalkParallel(root, 1, func(*WalkEntry) error { return nil })
	if ents, err := ReadDirents(root); err == nil && len(ents) >= 2 {
		_ = NameSort(ents[0], ents[1])
		_ = NameSort(ents[1], ents[0])
		_ = NameSort(ents[0], ents[0])
	}
	if names, err := ReadDirnames(root, NewScratchBuffer()); err == nil {
		_ = names
	}
	if ds, err := NewDirScanner(root); err == nil {
		for ds.Scan() {
			_, _ = ds.Dirent()
		}
		_ = ds.Err()
	}
	_ = GlobalRuntime()
	_ = DedicatedRuntime(1)
	_ = OwnedRuntime(nil)
	mw := NewParallelMultiWalker(root).AddRoot(root).Options(DefaultWalkOptions()).WithParallelism(1)
	_, _ = mw.Walk()
	_, _ = mw.Visit(func(ParallelMultiWalkEvent) WalkControl { return WalkContinue })
	_ = NewWalkBuilder(root)
	_ = (*File)(nil).Path()
	_ = (&File{Filename: "x"}).Path()
	_ = (&File{Location: root, Filename: "a.go"}).Path()
	ch := make(chan *File, 8)
	fw := NewParallelFileWalker(root, ch, 1).AddRoot(root).SetConcurrency(1).IgnoreGitignore().RespectGitModules()
	fw.Terminate()
	_ = fw.Start()
	multi, err := NewMultiScanner(root).Options(DefaultOptions()).WithRootParallelism(1).Scan(context.Background())
	if err != nil || multi.Len() != 1 {
		t.Fatal(err)
	}
	if _, err := NewMultiScanner(root).VisitContent(context.Background(), func(root, worker int) ContentVisitor {
		return func(ContentVisitEvent) ContentVisitControl { return ContentVisitContinue }
	}); err != nil {
		t.Fatal(err)
	}
	missing, err := NewScanner(filepath.Join(root, "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := missing.Scan(context.Background()); err == nil {
		t.Fatal("wrap")
	}
}

func TestVisitContentManifestSamePass(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	mustWriteFile(t, path, "one")
	scanner, err := NewScanner(root, WithOptions(DefaultOptions()))
	if err != nil {
		t.Fatal(err)
	}
	compact, err := scanner.VisitContentManifest(context.Background(), func(int) ContentVisitor {
		return func(ev ContentVisitEvent) ContentVisitControl {
			if ev.FileEnd {
				_ = os.WriteFile(path, []byte("two"), 0o644)
			}
			return ContentVisitContinue
		}
	})
	if err != nil || compact == nil || len(compact.Files) != 1 {
		t.Fatalf("%+v %v", compact, err)
	}
	if compact.Files[0].ContentHash != "sha256:" {
		if compact.Revision == "" {
			t.Fatal("empty revision")
		}
	}
	fresh, err := scanner.ScanCompact(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if compact.Revision == fresh.Revision && compact.Files[0].ContentHash != fresh.Files[0].ContentHash {
		t.Fatal("manifest reused a later scan revision")
	}
}

func TestCacheAdmissionFollowsPolicy(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "nul.bin"), "a\x00b")
	hashed, err := NewScanner(root, WithOptions(DefaultOptions().MetadataOnly().WithMaxFileBytes(0)))
	if err != nil {
		t.Fatal(err)
	}
	firstOpts := DefaultOptions()
	firstOpts.DetectBinaryFiles = false
	first, err := NewScanner(root, WithOptions(firstOpts))
	if err != nil {
		t.Fatal(err)
	}
	report, err := first.Scan(context.Background())
	if err != nil || len(report.Files) != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	cache := report.ToCache()
	detect := DefaultOptions()
	detect.DetectBinaryFiles = true
	second, err := NewScanner(root, WithOptions(detect))
	if err != nil {
		t.Fatal(err)
	}
	cached, err := second.ScanCached(context.Background(), &cache)
	if err != nil {
		t.Fatal(err)
	}
	if cached.Cache.ReusedHashes != 0 {
		t.Fatalf("binary policy reused unverified cache: %+v", cached)
	}
	meta, err := hashed.ScanCached(context.Background(), &cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Files) == 1 && meta.Files[0].ContentHash != "" {
		t.Fatal("metadata-only reused hash")
	}
}

func TestPublicJSONGoldens(t *testing.T) {
	ns := uint64(1700000000000000000)
	file := ScannedFile{
		Absolute: "/repo/a.go", Relative: "a.go",
		ContentHash:        "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ContentFingerprint: "fp128:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Bytes:              12, Version: FileVersion{ModifiedNS: &ns}, BinaryChecked: true,
	}
	cache := ScanCache{
		FormatVersion: 2, Root: "/repo",
		Entries: []ScanCacheEntry{{
			Relative:    "a.go",
			ContentHash: file.ContentHash, ContentFingerprint: file.ContentFingerprint,
			Bytes: 12, Version: FileVersion{ModifiedNS: &ns}, BinaryChecked: true,
		}},
	}
	assertJSONGolden(t, file, "../testdata/json/scanned_file.json")
	assertJSONGolden(t, cache, "../testdata/json/scan_cache.json")
}

func assertJSONGolden(t *testing.T, value any, path string) {
	t.Helper()
	got, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want = bytes.TrimSpace(want)
	if !bytes.Equal(got, want) {
		t.Fatalf("%s\ngot  %s\nwant %s", path, got, want)
	}
}

func TestExplainWinningIgnore(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".gitignore"), "# keep\ngenerated/**\n")
	mustMkdir(t, filepath.Join(root, "generated"))
	mustWriteFile(t, filepath.Join(root, "generated", "model.go"), "package g\n")
	mustWriteFile(t, filepath.Join(root, "keep.go"), "package keep\n")
	s, err := NewScanner(root)
	if err != nil {
		t.Fatal(err)
	}
	ex, err := s.Explain("generated/model.go")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Outcome != "excluded" || ex.Reason != "ignore_rule" || ex.Source != ".gitignore" || ex.Pattern != "generated/**" || ex.Line != 2 {
		t.Fatalf("%+v", ex)
	}
	keep, err := s.Explain("keep.go")
	if err != nil || keep.Outcome != "selected" {
		t.Fatalf("%+v %v", keep, err)
	}
}

func TestScanPathsOverrideScope(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "keep"))
	mustMkdir(t, filepath.Join(root, "vendor", "pkg"))
	mustWriteFile(t, filepath.Join(root, "keep", "a.txt"), "a")
	mustWriteFile(t, filepath.Join(root, "vendor", "pkg", "x.go"), "x")
	opts := DefaultOptions()
	opts.OverrideRules = []string{"keep/**"}
	s, err := NewScanner(root, WithOptions(opts))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := s.ScanPaths(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !containsRel(paths, "keep/a.txt") || strings.Contains(strings.Join(paths, "\n"), "vendor") {
		t.Fatalf("%v", paths)
	}
}

func TestIdentityReusesHash(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	mustWriteFile(t, a, "same-bytes")
	if err := os.Link(a, filepath.Join(root, "b.txt")); err != nil {
		t.Skip(err)
	}
	opts := DefaultOptions()
	opts.ContentWorkers = 1
	s, err := NewScanner(root, WithOptions(opts))
	if err != nil {
		t.Fatal(err)
	}
	report, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ha, hb string
	for _, f := range report.Files {
		switch f.Relative {
		case "a.txt":
			ha = f.ContentHash
		case "b.txt":
			hb = f.ContentHash
		}
	}
	if ha == "" || ha != hb {
		t.Fatalf("hash %q %q files=%d", ha, hb, len(report.Files))
	}
	if report.Cache.ReusedHashes == 0 {
		t.Fatalf("reused %+v", report.Cache)
	}
}

func TestTreeSnapshotApplyMatchesRebuild(t *testing.T) {
	files := []ScannedFile{
		{Relative: "a.go", ContentHash: "ha", Bytes: 1},
		{Relative: "b.go", ContentHash: "hb", Bytes: 2},
	}
	snap := SnapshotFromFiles(files)
	if snap.Len() != 2 || !strings.HasPrefix(snap.TreeRevision(), "tree2:") {
		t.Fatalf("%d %s", snap.Len(), snap.TreeRevision())
	}
	next := snap.Apply([]ScannedFile{{Relative: "b.go", ContentHash: "hb2", Bytes: 3}}, nil)
	rebuilt := SnapshotFromFiles([]ScannedFile{files[0], {Relative: "b.go", ContentHash: "hb2", Bytes: 3}})
	if next.TreeRevision() != rebuilt.TreeRevision() {
		t.Fatalf("%s vs %s", next.TreeRevision(), rebuilt.TreeRevision())
	}
	if next.TreeRevision() == snap.TreeRevision() {
		t.Fatal("legacy-style full rewrite was not required, but revision must change")
	}
}

func TestToScanOptionsKeepsWalkFollowWhenMaxOpenZero(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.txt"), "abc")
	opts := DefaultOptions()
	opts.Walk.FollowLinks = true
	opts.Walk.CollectMetadata = true
	opts.Walk.MaxOpen = 0
	report, err := ScanWith(context.Background(), root, Using(opts))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Files) != 1 {
		t.Fatalf("%+v", report.Files)
	}
	_ = walk.DefaultMaxOpen
}
