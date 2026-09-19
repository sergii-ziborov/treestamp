package fastwalk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sergii-ziborov/treestamp"
)

func TestWalkListsChildren(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustWrite(t, filepath.Join(root, "z.txt"), "z")
	var names []string
	err := Walk(nil, root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		names = append(names, d.Name())
		if _, ok := d.(DirEntry); !ok {
			t.Fatal("callback entry is not DirEntry")
		}
		return nil
	})
	if err != nil || len(names) != 2 {
		t.Fatalf("%v %v", names, err)
	}
}

func TestWalkSkipAllIsError(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	err := Walk(nil, root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		return fs.SkipAll
	})
	if !errors.Is(err, fs.SkipAll) {
		t.Fatalf("SkipAll %v", err)
	}
}

func TestWalkLexicalLocalOrder(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"c.txt", "a.txt", "b.txt"} {
		mustWrite(t, filepath.Join(root, name), "x")
	}
	var names []string
	err := Walk(&Config{NumWorkers: 2, Sort: SortLexical}, root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		names = append(names, d.Name())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "a.txt,b.txt,c.txt" {
		t.Fatalf("%q", names)
	}
}

func TestWalkKeepsSavedEntries(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	var saved fs.DirEntry
	err := Walk(nil, root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		saved = d
		return nil
	})
	if err != nil || saved == nil || saved.Name() != "a.txt" {
		t.Fatalf("saved=%v err=%v", saved, err)
	}
}

func TestWalkSkipFiles(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		mustWrite(t, filepath.Join(root, name), "x")
	}
	var n int
	err := Walk(&Config{NumWorkers: 1, Sort: SortLexical}, root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		n++
		return ErrSkipFiles
	})
	if err != nil || n != 1 {
		t.Fatalf("files=%d err=%v", n, err)
	}
}

func TestWalkMissingRoot(t *testing.T) {
	err := Walk(nil, filepath.Join(t.TempDir(), "gone"), func(string, fs.DirEntry, error) error { return nil })
	if err == nil {
		t.Fatal("expected stat error")
	}
}

func TestConfigCopyAndDefaults(t *testing.T) {
	src := &Config{Follow: true, Sort: SortFilesFirst, NumWorkers: 3}
	dupe := src.Copy()
	if dupe == src || !dupe.Follow || dupe.Sort != SortFilesFirst || dupe.NumWorkers != 3 {
		t.Fatalf("%+v", dupe)
	}
	n := DefaultNumWorkers()
	if n < 4 || n > 32 {
		t.Fatalf("workers %d", n)
	}
	if SortLexical.String() != "Lexical" {
		t.Fatal(SortLexical)
	}
}

func TestIgnoreDuplicateHardlink(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	mustWrite(t, a, "same")
	if err := os.Link(a, filepath.Join(root, "b.txt")); err != nil {
		t.Skip(err)
	}
	var n int
	err := Walk(nil, root, IgnoreDuplicateFiles(func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		n++
		return nil
	}))
	if err != nil || n != 1 {
		t.Fatalf("files=%d err=%v", n, err)
	}
}

func TestLegacySortBoolStillSerial(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "b", "x.txt"), "x")
	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	var names []string
	err := treestamp.WalkWithConfig(root, treestamp.Config{Sort: true}, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		if !d.IsDir() {
			names = append(names, d.Name())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "a.txt,x.txt" {
		t.Fatalf("legacy global sort %q", names)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
