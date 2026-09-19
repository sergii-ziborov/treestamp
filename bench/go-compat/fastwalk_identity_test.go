package compat_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/charlievieth/fastwalk"
	"github.com/sergii-ziborov/treestamp"
	tsfastwalk "github.com/sergii-ziborov/treestamp/compat/fastwalk"
)

func TestIgnoreDuplicateFilesHardlinkParity(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	if err := os.WriteFile(a, []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(a, filepath.Join(root, "b.txt")); err != nil {
		t.Skip(err)
	}
	want := collectDupFiles(t, root, func(root string, fn fs.WalkDirFunc) error {
		return fastwalk.Walk(nil, root, fn)
	}, fastwalk.IgnoreDuplicateFiles)
	got := collectDupFiles(t, root, treestamp.Walk, treestamp.IgnoreDuplicateFiles)
	if len(want) != 1 || len(got) != 1 {
		t.Fatalf("hardlink files fastwalk=%v treestamp=%v", want, got)
	}
}

func TestCompatFastwalkNodeSet(t *testing.T) {
	root := t.TempDir()
	mustWriteAll(t, root, []string{"a.txt", "b/c.txt", "z.txt"})
	want := collectWalkNames(t, func(fn fs.WalkDirFunc) error {
		return fastwalk.Walk(nil, root, fn)
	})
	got := collectWalkNames(t, func(fn fs.WalkDirFunc) error {
		return tsfastwalk.Walk(nil, root, fn)
	})
	if !equalStrings(want, got) {
		t.Fatalf("nodes fastwalk=%v compat=%v", want, got)
	}
}

func TestCompatFastwalkSkipAllError(t *testing.T) {
	root := t.TempDir()
	mustWriteAll(t, root, []string{"a.txt"})
	fwErr := fastwalk.Walk(nil, root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		return fs.SkipAll
	})
	tsErr := tsfastwalk.Walk(nil, root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		return fs.SkipAll
	})
	if !errors.Is(fwErr, fs.SkipAll) || !errors.Is(tsErr, fs.SkipAll) {
		t.Fatalf("SkipAll fastwalk=%v compat=%v", fwErr, tsErr)
	}
}

func collectDupFiles(t *testing.T, root string, walk func(string, fs.WalkDirFunc) error, wrap func(fs.WalkDirFunc) fs.WalkDirFunc) []string {
	t.Helper()
	var names []string
	err := walk(root, wrap(func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		names = append(names, d.Name())
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	return names
}

func mustWriteAll(t *testing.T, root string, rels []string) {
	t.Helper()
	for _, rel := range rels {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func collectWalkNames(t *testing.T, walk func(fs.WalkDirFunc) error) []string {
	t.Helper()
	var names []string
	err := walk(func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		names = append(names, d.Name())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	return names
}
