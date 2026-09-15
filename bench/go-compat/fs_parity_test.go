package compat_test

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sergii-ziborov/treestamp"
)

func TestArbitraryFSWalkParity(t *testing.T) {
	fsys := virtualTree()
	want, wantErr := collectFSPaths(fsys, "repo", fs.WalkDir)
	got, gotErr := collectFSPaths(fsys, "repo", treestamp.WalkFS)
	if wantErr != nil || gotErr != nil {
		t.Fatalf("walk errors: standard=%v treestamp=%v", wantErr, gotErr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths differ\nwant: %#v\n got: %#v", want, got)
	}
}

func TestArbitraryFSControlParity(t *testing.T) {
	fsys := virtualTree()
	decide := func(name string, _ fs.DirEntry, _ error) error {
		if name == "repo/skip" {
			return fs.SkipDir
		}
		return nil
	}
	want, wantErr := collectControlledFS(fsys, "repo", fs.WalkDir, decide)
	got, gotErr := collectControlledFS(fsys, "repo", treestamp.WalkFS, decide)
	if wantErr != nil || gotErr != nil {
		t.Fatalf("walk errors: standard=%v treestamp=%v", wantErr, gotErr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("controlled paths differ\nwant: %#v\n got: %#v", want, got)
	}
}

func TestArbitraryFSPullWalker(t *testing.T) {
	sub, err := fs.Sub(virtualTree(), "repo")
	if err != nil {
		t.Fatal(err)
	}
	walker, err := treestamp.NewFSWalker(sub, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer walker.Close()
	var paths []string
	for {
		entry, nextErr := walker.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatal(nextErr)
		}
		paths = append(paths, entry.Path())
		if entry.Path() == "skip" {
			walker.SkipCurrentDir()
		}
	}
	want := []string{".", "a.txt", "dir", "dir/b.txt", "link", "skip"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestArbitraryFSDirFSSymlinkParity(t *testing.T) {
	base := t.TempDir()
	write(t, filepath.Join(base, "target", "file.txt"), "x")
	mustSymlink(t, "target", filepath.Join(base, "alias"))
	fsys := os.DirFS(base)
	paths := assertFSRootParity(t, fsys, ".")
	if hasFSEvent(paths, "alias/file.txt") {
		t.Fatal("WalkFS followed a child symlink")
	}

	mustSymlink(t, "target", filepath.Join(base, "root-link"))
	paths = assertFSRootParity(t, fsys, "root-link")
	if !hasFSEvent(paths, "root-link/file.txt") {
		t.Fatal("WalkFS did not follow the root symlink")
	}
}

type fsWalkOperation func(fs.FS, string, fs.WalkDirFunc) error

func collectFSPaths(fsys fs.FS, root string, operation fsWalkOperation) ([]string, error) {
	return collectControlledFS(fsys, root, operation, nil)
}

func collectControlledFS(fsys fs.FS, root string, operation fsWalkOperation, decide walkDecisionFS) ([]string, error) {
	var paths []string
	err := operation(fsys, root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, name+":"+entry.Type().String())
		if decide != nil {
			return decide(name, entry, nil)
		}
		return nil
	})
	return paths, err
}

func assertFSRootParity(t *testing.T, fsys fs.FS, root string) []string {
	t.Helper()
	want, wantErr := collectFSPaths(fsys, root, fs.WalkDir)
	got, gotErr := collectFSPaths(fsys, root, treestamp.WalkFS)
	if wantErr != nil || gotErr != nil {
		t.Fatalf("walk errors: standard=%v treestamp=%v", wantErr, gotErr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths differ\nwant: %#v\n got: %#v", want, got)
	}
	return got
}

func hasFSEvent(events []string, name string) bool {
	for _, event := range events {
		if strings.HasPrefix(event, name+":") {
			return true
		}
	}
	return false
}

type walkDecisionFS func(string, fs.DirEntry, error) error

func virtualTree() fstest.MapFS {
	return fstest.MapFS{
		"repo/a.txt":         {Data: []byte("a")},
		"repo/dir/b.txt":     {Data: []byte("b")},
		"repo/link":          {Mode: fs.ModeSymlink},
		"repo/skip/file.txt": {Data: []byte("skip")},
	}
}
