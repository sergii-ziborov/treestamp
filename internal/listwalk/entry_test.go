package listwalk

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

type countDent struct {
	name string
	typ  fs.FileMode
	err  error
	n    atomic.Int32
}

func (c *countDent) Name() string               { return c.name }
func (c *countDent) IsDir() bool                { return c.typ.IsDir() }
func (c *countDent) Type() fs.FileMode          { return c.typ }
func (c *countDent) Info() (fs.FileInfo, error) { c.n.Add(1); return nil, c.err }

func TestOwnKeepsName(t *testing.T) {
	e := Own("a.txt", "a.txt", 0, 1, nil)
	if err := Deliver(func(_ string, d fs.DirEntry, err error) error {
		if d.Name() != "a.txt" {
			t.Fatalf("name %q", d.Name())
		}
		return err
	}, e, false); err != nil {
		t.Fatal(err)
	}
	if e.Name() != "a.txt" {
		t.Fatalf("kept %q", e.Name())
	}
}

func TestEntryKeepsNameAfterReleaseOfOriginal(t *testing.T) {
	e := Acquire("a.txt", "a.txt", 0, 1, nil)
	saved := e.Clone()
	Release(e)
	if saved.Name() != "a.txt" {
		t.Fatalf("name %q", saved.Name())
	}
}

func TestEntryCachesInfoError(t *testing.T) {
	src := &countDent{name: "x", err: fs.ErrPermission}
	e := Acquire("x", "x", 0, 0, nil)
	e.Bind(src)
	if _, err := e.Info(); err != fs.ErrPermission {
		t.Fatalf("%v", err)
	}
	if _, err := e.Info(); err != fs.ErrPermission {
		t.Fatalf("%v", err)
	}
	if src.n.Load() != 1 {
		t.Fatalf("source calls %d", src.n.Load())
	}
}

func TestEntryConcurrentInfo(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := Acquire("a.txt", path, 0, 0, nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.Info(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestEntryCloneDoesNotCopyPoolState(t *testing.T) {
	e := Acquire("a.txt", "gone", 0, 0, nil)
	src := &countDent{name: "a.txt", err: fs.ErrPermission}
	e.Bind(src)
	_, _ = e.Info()
	saved := e.Clone()
	Release(e)
	if _, err := saved.Info(); err != fs.ErrPermission {
		t.Fatalf("%v", err)
	}
	if src.n.Load() != 1 {
		t.Fatalf("source calls %d", src.n.Load())
	}
}
