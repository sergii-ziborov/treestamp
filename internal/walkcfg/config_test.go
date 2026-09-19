package walkcfg

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
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
