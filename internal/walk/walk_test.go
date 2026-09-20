package walk

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/sergii-ziborov/treestamp/internal/platform"
	rtruntime "github.com/sergii-ziborov/treestamp/internal/runtime"
)

func TestWalkerDoesNotUseWalkDirWrapper(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "sub", "b.txt"), "b")
	walker, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	var files []string
	for {
		entry, err := walker.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if entry.IsFile() {
			files = append(files, entry.RelativePath())
		}
	}
	if len(files) != 2 {
		t.Fatalf("files=%v", files)
	}
}

func TestFileRoot(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "only.txt")
	mustWrite(t, file, "x")
	walker, err := New(file)
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].IsFile() || entries[0].RelativePath() != "" {
		t.Fatalf("entries=%v", summarize(entries))
	}
}

func TestMaxDepth(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "a", "b"))
	mustWrite(t, filepath.Join(root, "a", "b", "c.txt"), "c")
	maxDepth := 1
	walker, err := NewWithOptions(root, WalkOptions{MaxDepth: &maxDepth, MaxOpen: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Depth() > 1 && entry.IsFile() {
			t.Fatalf("file beyond max depth: %s", entry.RelativePath())
		}
		if entry.IsDir() && entry.Depth() == 1 && entry.SkipReason() != SkipMaxDepth {
			t.Fatalf("expected max_depth skip on %s", entry.RelativePath())
		}
	}
}

func TestMaxOpenStillVisitsAllFiles(t *testing.T) {
	root := t.TempDir()
	var want int
	current := root
	for i := 0; i < 8; i++ {
		current = filepath.Join(current, "d")
		mustMkdir(t, current)
		mustWrite(t, filepath.Join(current, "f.txt"), "x")
		want++
	}
	walker, err := NewWithOptions(root, WalkOptions{MaxOpen: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	var files int
	for _, entry := range entries {
		if entry.IsFile() {
			files++
		}
	}
	if files != want {
		t.Fatalf("files=%d want=%d", files, want)
	}
}

func TestSkipCurrentDir(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "keep"))
	mustWrite(t, filepath.Join(root, "keep", "a.txt"), "a")
	mustMkdir(t, filepath.Join(root, "skip"))
	mustWrite(t, filepath.Join(root, "skip", "b.txt"), "b")
	walker, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	var files []string
	for {
		entry, err := walker.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if entry.IsDir() && entry.FileName() == "skip" {
			walker.SkipCurrentDir()
		}
		if entry.IsFile() {
			files = append(files, filepath.ToSlash(entry.RelativePath()))
		}
	}
	if len(files) != 1 {
		t.Fatalf("files=%v", files)
	}
}

func TestMetadataBytes(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "data.bin"), "hello")
	opts := DefaultOptions()
	opts.CollectMetadata = true
	walker, err := NewWithOptions(root, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.IsFile() {
			if entry.Bytes() == nil || *entry.Bytes() != 5 {
				t.Fatalf("bytes=%v", entry.Bytes())
			}
			found = true
		}
	}
	if !found {
		t.Fatal("missing file")
	}
}

func TestWalkBuilderSortAndMultiRoot(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	mustWrite(t, filepath.Join(a, "z.txt"), "z")
	mustWrite(t, filepath.Join(a, "a.txt"), "a")
	mustWrite(t, filepath.Join(b, "m.txt"), "m")
	walker := NewBuilder(a).AddRoot(b).SortByFileName().Build()
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsFile() {
			files = append(files, entry.FileName())
		}
	}
	if len(files) != 3 || files[0] != "a.txt" || files[1] != "z.txt" || files[2] != "m.txt" {
		t.Fatalf("order=%v", files)
	}
}

func TestContentsFirst(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "sub", "f.txt"), "f")
	walker := NewBuilder(root).ContentsFirst(true).Build()
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		rel := filepath.ToSlash(entry.RelativePath())
		if rel == "" {
			rel = "."
		}
		names = append(names, rel)
	}
	if len(names) < 3 || names[len(names)-1] != "." {
		t.Fatalf("root should be last in contents-first, got %v", names)
	}
}

func TestFilterPrunesDirectory(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "keep"))
	mustWrite(t, filepath.Join(root, "keep", "ok.txt"), "ok")
	mustMkdir(t, filepath.Join(root, "drop"))
	mustWrite(t, filepath.Join(root, "drop", "no.txt"), "no")
	walker := NewBuilder(root).FilterEntry(func(entry *WalkEntry) bool {
		return entry.FileName() != "drop"
	}).Build()
	defer walker.Close()
	entries, err := Collect(walker)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.FileName() == "no.txt" || entry.FileName() == "drop" {
			t.Fatalf("filter leaked %s", entry.Path())
		}
	}
}

func TestParallelWalkAndPull(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "sub", "b.txt"), "b")
	var n int
	if err := WalkParallel(root, 2, func(*WalkEntry) error { n++; return nil }); err != nil {
		t.Fatal(err)
	}
	if n < 3 {
		t.Fatalf("parallel %d", n)
	}
	got, err := CollectParallel(root, 2, true)
	if err != nil || len(got) < 3 {
		t.Fatalf("collect %v %v", len(got), err)
	}
	iter := NewParallelWalker(root).IntoIterBounded(2)
	defer iter.Close()
	var pulled int
	for {
		_, err := iter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		pulled++
	}
	if pulled < 3 {
		t.Fatalf("pull %d", pulled)
	}
	ordered, err := NewParallelWalker(root).TryIntoIterOrderedBounded(2)
	if err != nil {
		t.Fatal(err)
	}
	defer ordered.Close()
	if _, err := ordered.Next(); err != nil && err != io.EOF {
		t.Fatal(err)
	}
}

func TestWalkOptionsNormalizeAndSkipString(t *testing.T) {
	opts := WalkOptions{MaxOpen: 0}
	got := opts.Normalize()
	if got.MaxOpen != 1 {
		t.Fatalf("maxopen %d", got.MaxOpen)
	}
	if SkipMaxDepth.String() != "max_depth" || SkipNone.String() != "" {
		t.Fatal(SkipMaxDepth.String())
	}
	if OpReadDirectory.String() == "" || DefaultOptions().MaxOpen != DefaultMaxOpen {
		t.Fatal("defaults")
	}
	ev := WalkEvent{Err: walkErr("p", 1, OpReadMetadata, io.ErrUnexpectedEOF)}
	if !ev.IsError() || ev.Err.Unwrap() == nil {
		t.Fatal("walk error")
	}
}

func TestEmptyRootRejected(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("empty root")
	}
}

func TestStatefulWalk(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	b := NewStatefulWalkBuilder[int, string](root, 1)
	w, err := b.ProcessReadDir(func(depth int, dir string, state *int, batch *[]StatefulResult[string]) {
		for i := range *batch {
			if (*batch)[i].Entry != nil {
				(*batch)[i].Entry.State = "ok"
			}
		}
		_, _, _ = depth, dir, state
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

func summarize(entries []*WalkEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.RelativePath())
	}
	return out
}

func TestParallelCollectAndBuilder(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustWrite(t, filepath.Join(root, "sub", "b.txt"), "b")
	got, err := CollectParallel(root, 2, true)
	if err != nil || len(got) < 3 {
		t.Fatalf("parallel %d %v", len(got), err)
	}
	var n int
	if err := WalkParallel(root, 2, func(*WalkEntry) error { n++; return nil }); err != nil || n < 3 {
		t.Fatalf("walkp %d %v", n, err)
	}
	b := NewBuilder(root).SortByFileName().ContentsFirst(true).SkipStdout(true)
	w := b.Build()
	defer w.Close()
	if _, err := w.Next(); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	opts := DefaultOptions()
	opts.FollowLinks = true
	opts.CollectMetadata = true
	if _, err := NewWithOptions(root, opts); err != nil {
		t.Fatal(err)
	}
	iter := NewParallelWalker(root).IntoIterBounded(2)
	defer iter.Close()
	if _, err := iter.Next(); err != nil && err != io.EOF {
		t.Fatal(err)
	}
}

func TestWalkErrorsAndSkipReasons(t *testing.T) {
	if SkipMaxDepth.String() != "max_depth" || SkipNone.String() != "" {
		t.Fatal("skip")
	}
	if SkipFileSystemBoundary.String() == "" || SkipPathEscape.String() == "" || SkipSymlinkLoop.String() == "" {
		t.Fatal("more skip")
	}
	opts := DefaultOptions()
	opts.ErrorPolicy = ErrorAbort
	opts.SameFileSystem = true
	opts.MinDepth = 0
	if opts.Normalize().MaxOpen < 1 {
		t.Fatal("norm")
	}
	if _, err := New(""); err == nil {
		t.Fatal("empty")
	}
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	w, err := NewWithOptions(root, WalkOptions{MaxOpen: 1, CollectMetadata: true, ErrorPolicy: ErrorContinue})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if w.Root() == "" {
		t.Fatal("root")
	}
	_ = w.Options()
	entry, err := w.Next()
	if err != nil {
		t.Fatal(err)
	}
	if entry != nil && entry.IsDir() {
		w.SkipCurrentDir()
	}
}

func TestBuilderParallelAndEntryAccessors(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustWrite(t, filepath.Join(root, "sub", "b.txt"), "b")
	other := t.TempDir()
	mustWrite(t, filepath.Join(other, "c.txt"), "c")
	opts := DefaultOptions()
	opts.CollectMetadata = true
	opts.FollowLinks = true
	opts.SameFileSystem = true
	b := NewBuilder(root).AddRoot(other).Options(opts).FilterDirectories(func(e *WalkEntry) bool {
		return e.FileName() != "never"
	})
	mw := b.Build()
	defer mw.Close()
	entry, err := mw.Next()
	if err != nil {
		t.Fatal(err)
	}
	_ = entry.Path()
	_ = entry.IsSymlink()
	_ = entry.Version()
	if entry.IsDir() {
		mw.SkipCurrentDir()
	}
	got, err := Collect(mw)
	if err != nil || len(got) == 0 {
		t.Fatalf("collect %d %v", len(got), err)
	}
	pw := NewParallelWalker(root).Options(opts).SkipStdout(true).Runtime(rtruntime.Global()).WithParallelism(2)
	if _, err := pw.Walk(); err != nil {
		t.Fatal(err)
	}
	iter := pw.IntoIterOrderedBounded(2)
	defer iter.Close()
	if _, err := iter.Next(); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	ns := uint64(1)
	chg := uint64(2)
	id := platform.Identity{FileSystem: 1, File: 2}
	if !((FileVersion{ModifiedNS: &ns}).Reusable(FileVersion{ModifiedNS: &ns})) {
		t.Fatal("reuse")
	}
	if (FileVersion{ModifiedNS: &ns, ChangedNS: &chg}).Reusable(FileVersion{ModifiedNS: &ns, ChangedNS: &ns}) {
		t.Fatal("changed")
	}
	if (FileVersion{ModifiedNS: &ns, Identity: &id}).Reusable(FileVersion{ModifiedNS: &ns, Identity: &platform.Identity{FileSystem: 9, File: 9}}) {
		t.Fatal("id")
	}
	var empty *WalkError
	if empty.Error() != "" {
		t.Fatal("nil err")
	}
	if !isEOF(io.EOF) || !isEOF(fs.ErrClosed) || isEOF(io.ErrUnexpectedEOF) {
		t.Fatal("eof")
	}
	_ = walkErr(root, 0, OpReadEntry, io.ErrUnexpectedEOF).Error()
	if !containsID([]platformID{{fs: 1, file: 2}}, id) || containsID(nil, id) {
		t.Fatal("contains")
	}
	sb := NewStatefulWalkBuilder[int, string](root, 1).Options(opts).WithParallelism(1).ProcessReadDir(func(int, string, *int, *[]StatefulResult[string]) {})
	par, err := sb.BuildParallelOrdered(2)
	if err != nil {
		t.Fatal(err)
	}
	defer par.Close()
	st, err := par.Next()
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Path()
	_ = st.Depth()
	_ = st.IsDir()
	st.SetReadChildren(true)
	var dead *ParallelStatefulWalker[string]
	if _, err := dead.Next(); err != io.EOF {
		t.Fatal(err)
	}
	_ = (&ParallelStatefulWalker[string]{}).Close()
}

func TestOpenCloseYieldAndOps(t *testing.T) {
	_ = errRootSymlink.Error()
	_ = WalkOperation(99).String()
	_ = OpReadEntry.String()
	_ = OpScheduleWorker.String()
	var empty *WalkError
	if empty.Unwrap() != nil {
		t.Fatal("unwrap")
	}
	_ = (&WalkEntry{}).RelativePath()
	_ = (&WalkEntry{path: ".", root: "."}).FileName()
	max := 0
	norm := WalkOptions{MinDepth: 3, MaxDepth: &max}.Normalize()
	if norm.MinDepth != 0 {
		t.Fatal(norm.MinDepth)
	}
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "a", "b", "c"))
	mustWrite(t, filepath.Join(root, "a", "b", "c", "d.txt"), "d")
	mustWrite(t, filepath.Join(root, "only.txt"), "o")
	w, err := NewWithOptions(root, WalkOptions{MaxOpen: 1, CollectMetadata: true, ErrorPolicy: ErrorAbort})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Next(); err != nil {
		t.Fatal(err)
	}
	_, _ = w.yieldError(walkErr(root, 1, OpReadDirectory, io.ErrUnexpectedEOF))
	_ = w.Close()
	cont, err := NewWithOptions(root, WalkOptions{MaxOpen: 1, CollectMetadata: true, ErrorPolicy: ErrorContinue})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := cont.Next(); err != nil {
			break
		}
	}
	_ = cont.Close()
	fileRoot := filepath.Join(root, "only.txt")
	fr, err := NewWithOptions(fileRoot, WalkOptions{CollectMetadata: true})
	if err != nil {
		t.Fatal(err)
	}
	defer fr.Close()
	if _, err := fr.Next(); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	closed := NewParallelWalker("").IntoIterOrderedBounded(1)
	_, _ = closed.Next()
	_ = WalkParallel(root, 0, func(*WalkEntry) error { return io.EOF })
	tok := &rtruntime.Token{}
	tok.Cancel()
	_, _ = NewParallelWalker(root).VisitWithToken(tok, func(WalkEvent) WalkControl { return WalkContinue })
	fsopts := DefaultOptions()
	fsopts.SameFileSystem = true
	fsopts.FollowLinks = true
	fsopts.CollectMetadata = true
	if pw := NewParallelWalker(root).Options(fsopts).SkipStdout(true); pw != nil {
		_, _ = pw.Walk()
	}
	if entries, err := CollectParallel(root, 0, false); err != nil || len(entries) == 0 {
		t.Fatal(len(entries), err)
	}
	_, _ = Collect(errWalker{})
	if err := os.Symlink(fileRoot, filepath.Join(root, "link.txt")); err == nil {
		opts := DefaultOptions()
		opts.FollowLinks = true
		opts.RootSymlinkPolicy = RootReject
		_, _ = NewWithOptions(filepath.Join(root, "link.txt"), opts)
		opts.RootSymlinkPolicy = RootFollow
		if lw, err := NewWithOptions(root, opts); err == nil {
			_, _ = lw.Next()
			lw.Close()
		}
	}
}

type errWalker struct{}

func (errWalker) Next() (*WalkEntry, error) { return nil, io.ErrUnexpectedEOF }

func TestParallelRootSkipAndMaxDepthZero(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustWrite(t, filepath.Join(root, "sub", "b.txt"), "b")
	var seen []string
	_, err := NewParallelWalker(root).Visit(func(ev WalkEvent) WalkControl {
		if ev.Entry != nil {
			seen = append(seen, ev.Entry.RelativePath())
		}
		return WalkSkip
	})
	if err != nil || len(seen) != 1 {
		t.Fatalf("root skip %v %v", seen, err)
	}
	zero := 0
	report, err := NewParallelWalker(root).Options(WalkOptions{MaxDepth: &zero}).Walk()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range report.Entries {
		if !e.IsDir() && e.Depth() > 0 {
			t.Fatalf("descended %+v", e.RelativePath())
		}
	}
}

type rankDent struct {
	name string
	mode fs.FileMode
}

func (r rankDent) Name() string               { return r.name }
func (r rankDent) IsDir() bool                { return r.mode.IsDir() }
func (r rankDent) Type() fs.FileMode          { return r.mode }
func (r rankDent) Info() (fs.FileInfo, error) { return nil, fs.ErrInvalid }

func TestDirentRankDirsFirst(t *testing.T) {
	dents := []os.DirEntry{
		rankDent{"a-link", fs.ModeSymlink},
		rankDent{"dir", fs.ModeDir},
		rankDent{"z-file.go", 0},
	}
	sortDirents(dents, LocalSortDirsFirst)
	if dents[0].Name() != "dir" || dents[1].Name() != "z-file.go" || dents[2].Name() != "a-link" {
		t.Fatalf("%s %s %s", dents[0].Name(), dents[1].Name(), dents[2].Name())
	}
}
