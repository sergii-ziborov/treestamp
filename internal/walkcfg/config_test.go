package walkcfg

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sergii-ziborov/treestamp/internal/platform"
	rtruntime "github.com/sergii-ziborov/treestamp/internal/runtime"
	"github.com/sergii-ziborov/treestamp/internal/walk"
)

type rejectExec struct{}

func (rejectExec) Parallelism() int               { return 1 }
func (rejectExec) TryExecute(rtruntime.Job) error { return rtruntime.ErrBusy }

func TestParallelSkipFilesUsesLockedMap(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt", "d.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var (
		mu    sync.Mutex
		files []string
		once  sync.Once
		gate  = make(chan struct{})
	)
	err := Parallel(root, Config{NumWorkers: 4, MaxDepth: 4}, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		once.Do(func() { close(gate) })
		<-gate
		mu.Lock()
		files = append(files, d.Name())
		mu.Unlock()
		return walk.ErrSkipFiles
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no files visited")
	}
}

func TestParallelLocalLexicalSort(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"c.txt", "a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var names []string
	err := Parallel(root, Config{NumWorkers: 2, SortMode: walk.LocalSortLexical}, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		names = append(names, d.Name())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 || names[0]+names[1]+names[2] != "a.txtb.txtc.txt" {
		t.Fatalf("%q", names)
	}
}

func TestParallelKeepSkipAll(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Parallel(root, Config{NumWorkers: 2, KeepSkipAll: true}, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		return fs.SkipAll
	})
	if err != fs.SkipAll {
		t.Fatalf("KeepSkipAll %v", err)
	}
	quiet := Parallel(root, Config{NumWorkers: 2}, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		return fs.SkipAll
	})
	if quiet != nil {
		t.Fatalf("native SkipAll %v", quiet)
	}
}

func TestParallelOwnedEntryKeepsName(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var saved fs.DirEntry
	err := Parallel(root, Config{NumWorkers: 2}, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		saved = d
		return nil
	})
	if err != nil || saved == nil || saved.Name() != "keep.txt" {
		t.Fatalf("saved=%v err=%v", saved, err)
	}
}

func TestParallelSkipFilesOnStream(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	err := Parallel(root, Config{NumWorkers: 1}, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		n++
		return walk.ErrSkipFiles
	})
	if err != nil || n != 1 {
		t.Fatalf("files=%d err=%v", n, err)
	}
}

func TestOrderedPullAdmitFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := rtruntime.Owned(rejectExec{}).WithAdmitTimeout(15 * time.Millisecond)
	_, err := walk.NewParallelWalker(root).Runtime(rt).TryIntoIterOrderedBounded(2)
	if !errors.Is(err, rtruntime.ErrAdmitTimeout) && !errors.Is(err, rtruntime.ErrBusy) {
		t.Fatalf("admit %v", err)
	}
}

func TestOrderedPullCloseUnblocks(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		name := filepath.Join(root, "d", "f"+string(rune('a'+i))+".txt")
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	iter, err := walk.NewParallelWalker(root).TryIntoIterOrderedBounded(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := iter.Next(); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- iter.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close hung")
	}
}

func TestParallelUsesWorkersBelowSingleChild(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		dir := filepath.Join(src, "d"+string(rune('0'+i)))
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var current, peak atomic.Int32
	err := Parallel(root, Config{NumWorkers: 4}, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		n := current.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		current.Add(-1)
		return nil
	})
	if err != nil || peak.Load() < 2 {
		t.Fatalf("peak=%d err=%v", peak.Load(), err)
	}
}

func TestParallelDirsFirstPutsRegularBeforeSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "z-file.go"), []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "z-file.go"), filepath.Join(root, "a-link")); err != nil {
		t.Skip(err)
	}
	var kids []string
	err := Parallel(root, Config{NumWorkers: 1, SortMode: walk.LocalSortDirsFirst}, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		if filepath.Dir(path) == root {
			kids = append(kids, d.Name())
		}
		return nil
	})
	want := []string{"dir", "z-file.go", "a-link"}
	if err != nil || len(kids) != 3 || kids[0] != want[0] || kids[1] != want[1] || kids[2] != want[2] {
		t.Fatalf("kids=%q err=%v", kids, err)
	}
}

func drainOrdered(t *testing.T, iter *walk.ParallelWalkIter) int {
	t.Helper()
	done := make(chan int, 1)
	go func() {
		n := 0
		for {
			_, err := iter.Next()
			if err == io.EOF {
				done <- n
				return
			}
			if err != nil {
				done <- -1
				return
			}
			n++
		}
	}()
	select {
	case n := <-done:
		if n < 0 {
			t.Fatal("ordered pull error")
		}
		return n
	case <-time.After(3 * time.Second):
		_ = iter.Close()
		t.Fatal("ordered pull hung")
	}
	return 0
}

func TestOrderedPullCapacityOneNested(t *testing.T) {
	for _, workers := range []int{1, 2} {
		t.Run(string(rune('0'+workers)), func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "sub", "file.txt"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			iter, err := walk.NewParallelWalker(root).WithParallelism(workers).TryIntoIterOrderedBounded(1)
			if err != nil {
				t.Fatal(err)
			}
			defer iter.Close()
			if n := drainOrdered(t, iter); n < 3 {
				t.Fatalf("entries=%d workers=%d", n, workers)
			}
		})
	}
}

func TestOrderedPullEmptyDirCapacityOne(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	iter, err := walk.NewParallelWalker(root).WithParallelism(1).TryIntoIterOrderedBounded(1)
	if err != nil {
		t.Fatal(err)
	}
	defer iter.Close()
	if n := drainOrdered(t, iter); n < 3 {
		t.Fatalf("entries=%d", n)
	}
}

func TestSerialSortModeDirsFirst(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "z-file.go"), []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a-file.go"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	var kids []string
	err := Serial(root, Config{SortMode: walk.LocalSortDirsFirst}, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		if filepath.Dir(path) == root {
			kids = append(kids, d.Name())
		}
		return nil
	})
	if err != nil || len(kids) != 3 || kids[0] != "dir" || kids[1] != "a-file.go" || kids[2] != "z-file.go" {
		t.Fatalf("kids=%q err=%v", kids, err)
	}
}

func TestIntoIterOrderedBoundedSurfacesAdmitError(t *testing.T) {
	rt := rtruntime.Owned(rejectExec{}).WithAdmitTimeout(15 * time.Millisecond)
	iter := walk.NewParallelWalker(t.TempDir()).Runtime(rt).IntoIterOrderedBounded(1)
	_, err := iter.Next()
	if iter.Err() == nil {
		t.Fatal("missing constructor error")
	}
	if !errors.Is(err, iter.Err()) {
		t.Fatalf("Next %v Err %v", err, iter.Err())
	}
	if !errors.Is(err, rtruntime.ErrAdmitTimeout) && !errors.Is(err, rtruntime.ErrBusy) {
		t.Fatalf("admit %v", err)
	}
}

func TestCollectMetadataKeepsEnumeration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "m.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := walk.DefaultOptions()
	opts.CollectMetadata = true
	w, err := walk.NewWithOptions(root, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	entries, err := walk.Collect(w)
	if err != nil {
		t.Fatal(err)
	}
	var file *walk.WalkEntry
	for _, entry := range entries {
		if entry.IsFile() {
			file = entry
			break
		}
	}
	if file == nil || file.Bytes() == nil || *file.Bytes() != 5 || file.Version() == nil || file.Version().ModifiedNS == nil {
		t.Fatalf("meta %+v", file)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	id, ok := platform.IdentityFromInfo(info)
	got := file.Version().Identity
	if ok {
		if got == nil || !got.Equal(id) {
			t.Fatalf("identity %v %v", got, id)
		}
		return
	}
	if got != nil {
		t.Fatal("must not invent identity from a second open")
	}
}
