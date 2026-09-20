package treestamp_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

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

func TestFileWalkerCustomIgnorePatternsIgnore(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "keep.go"), "package keep")
	mustWriteFile(t, filepath.Join(root, "drop.tmp"), "tmp")
	ch := make(chan *File, 8)
	w := NewFileWalker(root, ch)
	w.CustomIgnorePatterns = []string{"*.tmp"}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	var names []string
	for file := range ch {
		names = append(names, file.Filename)
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "keep.go") || strings.Contains(joined, "drop.tmp") {
		t.Fatalf("%q", names)
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

func TestScanFSIgnoreHashAndEachFile(t *testing.T) {
	fsys := fstest.MapFS{
		".gitignore":         {Data: []byte("secret.bin\n")},
		"keep.go":            {Data: []byte("package keep\n")},
		"secret.bin":         {Data: []byte("hidden")},
		"node_modules/x.js":  {Data: []byte("x")},
		"nested/.gitignore":  {Data: []byte("drop.txt\n")},
		"nested/keep.txt":    {Data: []byte("ok")},
		"nested/drop.txt":    {Data: []byte("no")},
	}
	ctx := context.Background()
	paths, err := ScanPathsFS(ctx, fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(paths, "\n")
	if strings.Contains(joined, "secret.bin") || strings.Contains(joined, "node_modules") || strings.Contains(joined, "drop.txt") {
		t.Fatalf("leaked %v", paths)
	}
	if !containsRel(paths, "keep.go") || !containsRel(paths, "nested/keep.txt") {
		t.Fatalf("missing %v", paths)
	}
	rep, err := ScanFS(ctx, fsys, ".")
	if err != nil || rep == nil || !rep.Portable || rep.Revision == "" {
		t.Fatalf("%+v %v", rep, err)
	}
	var selected []string
	for _, file := range rep.Files {
		selected = append(selected, file.Relative)
	}
	got := strings.Join(selected, "\n")
	if strings.Contains(got, "secret.bin") || strings.Contains(got, "drop.txt") || strings.Contains(got, "node_modules") {
		t.Fatalf("hashed leak %v", selected)
	}
	if !containsRel(selected, "keep.go") || !containsRel(selected, "nested/keep.txt") {
		t.Fatalf("hashed missing %v", selected)
	}
	var saw int
	sum, err := EachFileFS(ctx, fsys, ".", func(f ScannedFile, data []byte) error {
		saw++
		if f.ContentHash == "" || data == nil {
			t.Fatalf("%+v %q", f, data)
		}
		return nil
	})
	if err != nil || saw != len(rep.Files) || sum == nil {
		t.Fatalf("each %d want %d %v %v", saw, len(rep.Files), sum, err)
	}
}

func TestScanFSGrowingFileIsConcurrent(t *testing.T) {
	fsys := growFS{name: "grow.txt", stat: 4, body: []byte("abcdef")}
	rep, err := ScanFS(context.Background(), fsys, ".")
	if err != nil && !errors.Is(err, ErrPartial) {
		t.Fatal(err)
	}
	if rep == nil {
		t.Fatal("nil report")
	}
	for _, skipped := range rep.Skipped {
		if skipped.Kind == SkipConcurrentModification && skipped.Relative == "grow.txt" {
			return
		}
	}
	t.Fatalf("expected concurrent_modification %+v files=%+v", rep.Skipped, rep.Files)
}

func TestMultiScanReportRevisionStable(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	mustWriteFile(t, filepath.Join(a, "a.go"), "package a\n")
	mustWriteFile(t, filepath.Join(b, "b.go"), "package b\n")
	first, err := NewMultiScanner(a).AddRoot(b).Scan(context.Background())
	if err != nil || first == nil || first.Len() != 2 {
		t.Fatalf("%v %v", first, err)
	}
	rev := first.Revision()
	if rev == "" || !strings.HasPrefix(rev, "sha256:") {
		t.Fatalf("rev %q", rev)
	}
	again, err := NewMultiScanner(a).AddRoot(b).Scan(context.Background())
	if err != nil || again.Revision() != rev {
		t.Fatalf("unstable %q %q %v", rev, again.Revision(), err)
	}
	swapped, err := NewMultiScanner(b).AddRoot(a).Scan(context.Background())
	if err != nil || swapped.Revision() == rev {
		t.Fatalf("order ignored %q %q", rev, swapped.Revision())
	}
}

type growFS struct {
	name string
	stat int64
	body []byte
}

func (g growFS) Open(name string) (fs.File, error) {
	if name == "." || name == "" {
		return &growDir{names: []string{g.name}}, nil
	}
	if name != g.name {
		return nil, fs.ErrNotExist
	}
	return &growFile{info: growInfo{name: g.name, size: g.stat}, data: g.body}, nil
}

type growDir struct {
	names []string
	off   int
}

func (d *growDir) Stat() (fs.FileInfo, error) {
	return growInfo{name: ".", size: 0, mode: fs.ModeDir}, nil
}
func (d *growDir) Read([]byte) (int, error) { return 0, fs.ErrInvalid }
func (d *growDir) Close() error             { return nil }
func (d *growDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.off >= len(d.names) {
		return nil, io.EOF
	}
	if n <= 0 {
		n = len(d.names) - d.off
	}
	var out []fs.DirEntry
	for i := 0; i < n && d.off < len(d.names); i++ {
		out = append(out, growInfo{name: d.names[d.off], size: 0})
		d.off++
	}
	return out, nil
}

type growFile struct {
	info growInfo
	data []byte
	off  int
}

func (f *growFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *growFile) Close() error               { return nil }
func (f *growFile) Read(p []byte) (int, error) {
	if f.off >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.off:])
	f.off += n
	return n, nil
}

type growInfo struct {
	name string
	size int64
	mode fs.FileMode
}

func (i growInfo) Name() string       { return i.name }
func (i growInfo) Size() int64        { return i.size }
func (i growInfo) Mode() fs.FileMode  { return i.mode }
func (i growInfo) ModTime() time.Time { return time.Time{} }
func (i growInfo) IsDir() bool        { return i.mode.IsDir() }
func (i growInfo) Sys() any           { return nil }
func (i growInfo) Type() fs.FileMode  { return i.mode.Type() }
func (i growInfo) Info() (fs.FileInfo, error) {
	if i.size == 0 && !i.mode.IsDir() {
		return growInfo{name: i.name, size: 4}, nil
	}
	return i, nil
}
